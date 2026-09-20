package areafault_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/areafault"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/lamp"
)

type harness struct {
	lamps  *lamp.Service
	faults *fault.Service
	areas  *areafault.Service
	db     *gorm.DB
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&lamp.Lamp{}, &fault.Fault{}, &areafault.AreaFault{}, &areafault.AreaFaultItem{}))

	lampRepository := lamp.NewRepository(db)
	lampService := lamp.NewService(lampRepository)

	faultRepository := fault.NewRepository(db)
	faultService := fault.NewService(faultRepository, lampService)
	lampService.SetOpenFaultCounter(faultRepository)

	areaModule := areafault.New(db, lampService, faultService)
	lampService.SetOpenAreaFaultCounter(areaModule.Repository())

	return &harness{lamps: lampService, faults: faultService, areas: areaModule.Service(), db: db}
}

func (h *harness) createLamp(t *testing.T, code string) *lamp.Lamp {
	t.Helper()
	entity, err := h.lamps.Create(context.Background(), lamp.CreateRequest{
		Code: code, Name: "测试灯杆", RoadName: "测试路", LampType: lamp.LampTypeLED,
	})
	require.NoError(t, err)
	return entity
}

func requireConflict(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	businessErr, ok := apperr.As(err)
	require.True(t, ok, "期望业务错误, 实际: %v", err)
	require.Equal(t, http.StatusConflict, businessErr.Status, "错误信息: %s", businessErr.Message)
}

func TestAreaFaultDispatchHandleAndClose(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	d1 := h.createLamp(t, "LD-A-001")
	d2 := h.createLamp(t, "LD-A-002")
	d3 := h.createLamp(t, "LD-A-003")
	ids := []uint{d1.ID, d2.ID, d3.ID}

	// 建立区域故障: 同一回路 3 盏路灯, 应同步为每盏灯各建一条待处理故障
	detail, err := h.areas.Create(ctx, areafault.CreateRequest{
		Cause: areafault.CauseLine, CircuitName: "测试回路-1", LampIDs: ids,
		FaultType: "线路故障", FaultLevel: fault.LevelUrgent, Description: "电缆击穿整段失电", Reporter: "监控中心",
	})
	require.NoError(t, err)
	require.Equal(t, areafault.StatusPending, detail.Status)
	require.Equal(t, 3, detail.TotalCount)
	require.Equal(t, 3, detail.PendingCount)
	require.Regexp(t, `^QY\d{8}\d{4}$`, detail.AreaNo)
	require.Len(t, detail.Items, 3)
	for _, item := range detail.Items {
		require.NotNil(t, item.FaultID)
		require.Equal(t, areafault.ItemResultPending, item.Result)
	}

	// 关联路灯在区域建单后应进入故障状态
	device, err := h.lamps.Get(ctx, d1.ID)
	require.NoError(t, err)
	require.Equal(t, lamp.RunStatusFault, device.RunStatus)

	// 未派工前不允许登记逐盏处置结果
	_, err = h.areas.HandleItem(ctx, detail.Items[0].ID, areafault.HandleItemRequest{Result: areafault.ItemResultRecovered})
	requireConflict(t, err)

	// 统一派工
	dispatched, err := h.areas.Dispatch(ctx, detail.ID, areafault.DispatchRequest{Assignee: "周涛", RepairTeam: "市政照明二班"})
	require.NoError(t, err)
	require.Equal(t, areafault.StatusDispatched, dispatched.Status)
	require.NotNil(t, dispatched.DispatchedAt)

	// 第一盏恢复
	afterFirst, err := h.areas.HandleItem(ctx, dispatched.Items[0].ID, areafault.HandleItemRequest{
		Result: areafault.ItemResultRecovered, Remark: "更换接头恢复",
	})
	require.NoError(t, err)
	require.Equal(t, 1, afterFirst.RecoveredCount)
	require.Equal(t, 2, afterFirst.PendingCount)
	require.Equal(t, areafault.StatusDispatched, afterFirst.Status, "仍有遗留时不应全部恢复")

	// 第二盏登记为待配件(遗留)
	afterParts, err := h.areas.HandleItem(ctx, dispatched.Items[1].ID, areafault.HandleItemRequest{
		Result: areafault.ItemResultParts, Remark: "端子缺货",
	})
	require.NoError(t, err)
	require.Equal(t, 1, afterParts.RecoveredCount)
	require.Equal(t, 2, afterParts.PendingCount)

	// 未全部恢复前不允许整体闭环
	_, err = h.areas.Close(ctx, detail.ID, areafault.CloseRequest{Remark: "闭环"})
	requireConflict(t, err)

	// 第三盏恢复, 但第二盏仍遗留 -> 仍不可闭环
	afterThird, err := h.areas.HandleItem(ctx, dispatched.Items[2].ID, areafault.HandleItemRequest{
		Result: areafault.ItemResultRecovered, Remark: "送电正常",
	})
	require.NoError(t, err)
	require.Equal(t, 2, afterThird.RecoveredCount)
	require.Equal(t, 1, afterThird.PendingCount)
	_, err = h.areas.Close(ctx, detail.ID, areafault.CloseRequest{})
	requireConflict(t, err)

	// 第二盏补料后改判为已恢复 -> 全部恢复
	restored, err := h.areas.HandleItem(ctx, dispatched.Items[1].ID, areafault.HandleItemRequest{
		Result: areafault.ItemResultRecovered, Remark: "端子到货更换完成",
	})
	require.NoError(t, err)
	require.Equal(t, areafault.StatusRestored, restored.Status)
	require.Equal(t, 0, restored.PendingCount)
	require.NotNil(t, restored.AllRestoredAt)
	require.NotNil(t, restored.RestoreMinutes)

	// 全部恢复后闭环
	closed, err := h.areas.Close(ctx, detail.ID, areafault.CloseRequest{Remark: "逐盏复核完成, 闭环"})
	require.NoError(t, err)
	require.Equal(t, areafault.StatusClosed, closed.Status)
	require.NotNil(t, closed.ClosedAt)

	// 已闭环不允许重复闭环
	_, err = h.areas.Close(ctx, detail.ID, areafault.CloseRequest{})
	requireConflict(t, err)
}

func TestAreaFaultRejectsDuplicateOpenLamp(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d1 := h.createLamp(t, "LD-B-001")
	d2 := h.createLamp(t, "LD-B-002")

	_, err := h.faults.Create(ctx, fault.CreateRequest{
		LampID: d1.ID, FaultType: "灯不亮", Description: "已有未闭环单灯故障",
	})
	require.NoError(t, err)

	// 回路上存在已有未闭环故障的路灯时, 不允许建立区域故障
	_, err = h.areas.Create(ctx, areafault.CreateRequest{
		Cause: areafault.CauseCabinet, CircuitName: "测试回路-2", LampIDs: []uint{d1.ID, d2.ID},
		FaultType: "控制箱故障", Description: "控制箱停电",
	})
	requireConflict(t, err)
}

func TestCircuitOverview(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d1 := h.createLamp(t, "LD-C-001")
	d2 := h.createLamp(t, "LD-C-002")

	detail, err := h.areas.Create(ctx, areafault.CreateRequest{
		Cause: areafault.CauseLine, CircuitName: "概览回路", LampIDs: []uint{d1.ID, d2.ID},
		FaultType: "线路故障", Description: "概览测试",
	})
	require.NoError(t, err)
	_, err = h.areas.Dispatch(ctx, detail.ID, areafault.DispatchRequest{Assignee: "周涛"})
	require.NoError(t, err)
	dispatched, err := h.areas.Get(ctx, detail.ID)
	require.NoError(t, err)
	_, err = h.areas.HandleItem(ctx, dispatched.Items[0].ID, areafault.HandleItemRequest{Result: areafault.ItemResultRecovered})
	require.NoError(t, err)

	overview, err := h.areas.CircuitOverview(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), overview.OpenTotal)
	require.Equal(t, int64(2), overview.AffectedLamps)
	require.Equal(t, int64(1), overview.RecoveredLamps)
	require.Equal(t, int64(1), overview.PendingLamps)
	require.Len(t, overview.Circuits, 1)
	require.Equal(t, "概览回路", overview.Circuits[0].CircuitName)
	require.Nil(t, overview.Circuits[0].AvgRestoreMinutes, "未全部恢复时无恢复耗时样本")
}
