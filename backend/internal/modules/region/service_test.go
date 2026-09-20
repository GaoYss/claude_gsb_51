package region_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/lamp"
	"streetlight/internal/modules/region"
	"streetlight/internal/modules/repair"
)

type harness struct {
	db      *gorm.DB
	lamps   *lamp.Service
	faults  *fault.Service
	repairs *repair.Service
	regions *region.Service
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

	require.NoError(t, db.AutoMigrate(&lamp.Lamp{}, &fault.Fault{}, &repair.Repair{}, &region.RegionFault{}))

	lampRepository := lamp.NewRepository(db)
	lampService := lamp.NewService(lampRepository)

	faultRepository := fault.NewRepository(db)
	faultService := fault.NewService(faultRepository, lampService)
	lampService.SetOpenFaultCounter(faultRepository)

	repairRepository := repair.NewRepository(db)
	repairService := repair.NewService(repairRepository, faultService)

	regionService := region.NewService(
		region.NewRepository(db), lampRepository, faultService, repairService, repairRepository,
	)

	return &harness{
		db:      db,
		lamps:   lampService,
		faults:  faultService,
		repairs: repairService,
		regions: regionService,
	}
}

func (h *harness) createLampWithCircuit(t *testing.T, code, road, circuit string) *lamp.Lamp {
	t.Helper()
	entity, err := h.lamps.Create(context.Background(), lamp.CreateRequest{
		Code:        code,
		Name:        code + "号灯杆",
		RoadName:    road,
		CircuitCode: circuit,
		LampType:    lamp.LampTypeLED,
	})
	require.NoError(t, err)
	return entity
}

func (h *harness) createLamp(t *testing.T, code string) *lamp.Lamp {
	t.Helper()
	return h.createLampWithCircuit(t, code, "测试路", "WL-TEST-01")
}

func requireStatus(t *testing.T, err error, status int) {
	t.Helper()
	require.Error(t, err)
	businessErr, ok := apperr.As(err)
	require.True(t, ok, "期望业务错误, 实际: %v", err)
	require.Equal(t, status, businessErr.Status, "错误信息: %s", businessErr.Message)
}

func buildCreateReq(lampIDs []uint, circuit string) region.CreateRequest {
	return region.CreateRequest{
		CircuitCode: circuit,
		LampIDs:     lampIDs,
		Cause:       region.CauseLine,
		FaultLevel:  fault.LevelHigh,
		Description: "整条回路失电",
		Reporter:    "监控中心",
	}
}

func nowMinus(d time.Duration) string {
	return time.Now().Add(-d).Format("2006-01-02 15:04:05")
}

func TestRegionDispatchResolveAndClose(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	l1 := h.createLamp(t, "LD-R-001")
	l2 := h.createLamp(t, "LD-R-002")
	l3 := h.createLamp(t, "LD-R-003")

	entity, err := h.regions.Create(ctx, buildCreateReq([]uint{l1.ID, l2.ID, l3.ID}, ""))
	require.NoError(t, err)
	require.Equal(t, region.StatusPending, entity.Status)
	require.Equal(t, 3, entity.AffectedCount)
	require.Equal(t, 0, entity.RestoredCount)
	require.Regexp(t, `^QY\d{8}\d{4}$`, entity.No)

	// 每盏路灯都生成一条待处理子故障, 路灯状态联动为故障
	children, err := h.faults.Repository().ListByRegion(ctx, entity.ID)
	require.NoError(t, err)
	require.Len(t, children, 3)
	for _, child := range children {
		require.Equal(t, fault.StatusPending, child.Status)
		require.Equal(t, region.CauseLine, child.FaultType)
	}
	device, err := h.lamps.Get(ctx, l1.ID)
	require.NoError(t, err)
	require.Equal(t, lamp.RunStatusFault, device.RunStatus)

	// 未派工前不允许闭环
	_, err = h.regions.Close(ctx, entity.ID, region.CloseRequest{})
	requireStatus(t, err, http.StatusConflict)

	// 统一派工: 三盏灯同时开工
	_, err = h.regions.Dispatch(ctx, entity.ID, region.DispatchRequest{
		Repairman: "陈鹏", RepairTeam: "市政照明二班",
	})
	require.NoError(t, err)

	updated, err := h.regions.Get(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, region.StatusProcessing, updated.Status)
	require.NotNil(t, updated.DispatchedAt)
	require.Equal(t, "陈鹏", updated.DispatchRepairman)

	children, err = h.faults.Repository().ListByRegion(ctx, entity.ID)
	require.NoError(t, err)
	for _, child := range children {
		require.Equal(t, fault.StatusProcessing, child.Status)
	}
	device, _ = h.lamps.Get(ctx, l1.ID)
	require.Equal(t, lamp.RunStatusMaintenance, device.RunStatus)

	// 重复派工被拒绝
	_, err = h.regions.Dispatch(ctx, entity.ID, region.DispatchRequest{Repairman: "其他人"})
	requireStatus(t, err, http.StatusConflict)

	// 逐盏登记: 前两盏已修复, 第三盏待配件
	detail, err := h.regions.Detail(ctx, entity.ID)
	require.NoError(t, err)
	require.Len(t, detail.Items, 3)

	fixedIDs := []uint{children[0].ID, children[1].ID}
	for _, faultID := range fixedIDs {
		_, err = h.regions.ResolveLamp(ctx, entity.ID, faultID, region.ResolveLampRequest{
			Result: repair.ResultFixed, Content: "更换电缆段",
		})
		require.NoError(t, err)
	}
	_, err = h.regions.ResolveLamp(ctx, entity.ID, children[2].ID, region.ResolveLampRequest{
		Result: repair.ResultPendingParts, Content: "等待电缆物料",
	})
	require.NoError(t, err)

	updated, _ = h.regions.Get(ctx, entity.ID)
	require.Equal(t, 2, updated.RestoredCount)
	require.Nil(t, updated.RestoredAt, "尚未全部恢复, 不应记录恢复时间")

	// 仍有一盏未恢复, 整体闭环被拒绝
	_, err = h.regions.Close(ctx, entity.ID, region.CloseRequest{})
	requireStatus(t, err, http.StatusConflict)

	// 已修复的子故障不能再次派工
	_, err = h.regions.RedispatchLamp(ctx, entity.ID, children[0].ID, region.RedispatchRequest{Repairman: "陈鹏"})
	requireStatus(t, err, http.StatusConflict)

	// 对第三盏再次派工(返修), 然后登记为已修复
	item, err := h.regions.RedispatchLamp(ctx, entity.ID, children[2].ID, region.RedispatchRequest{
		Repairman: "周涛", Content: "物料到场后重做接头",
	})
	require.NoError(t, err)
	require.Equal(t, repair.StatusOngoing, item.RepairStatus)

	_, err = h.regions.ResolveLamp(ctx, entity.ID, children[2].ID, region.ResolveLampRequest{
		Result: repair.ResultFixed, Content: "更换电缆并复测合格",
	})
	require.NoError(t, err)

	updated, _ = h.regions.Get(ctx, entity.ID)
	require.Equal(t, 3, updated.RestoredCount)
	require.NotNil(t, updated.RestoredAt, "全部恢复应记录恢复时间")

	// 全部恢复后允许整体闭环, 子故障一并关闭
	closed, err := h.regions.Close(ctx, entity.ID, region.CloseRequest{Remark: "回路恢复正常供电"})
	require.NoError(t, err)
	require.Equal(t, region.StatusClosed, closed.Status)
	require.NotNil(t, closed.ClosedAt)

	children, _ = h.faults.Repository().ListByRegion(ctx, entity.ID)
	for _, child := range children {
		require.Equal(t, fault.StatusClosed, child.Status)
	}
	device, _ = h.lamps.Get(ctx, l3.ID)
	require.Equal(t, lamp.RunStatusNormal, device.RunStatus)

	// 已闭环不允许重复闭环
	_, err = h.regions.Close(ctx, entity.ID, region.CloseRequest{})
	requireStatus(t, err, http.StatusConflict)
}

func TestRegionRejectsMixedCircuitAndUnavailableLamps(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	same1 := h.createLampWithCircuit(t, "LD-M-001", "解放路", "WL-JF-01")
	same2 := h.createLampWithCircuit(t, "LD-M-002", "解放路", "WL-JF-01")
	other := h.createLampWithCircuit(t, "LD-M-003", "中山路", "WL-ZS-02")

	// 跨回路关联被拒绝
	_, err := h.regions.Create(ctx, buildCreateReq([]uint{same1.ID, other.ID}, ""))
	requireStatus(t, err, http.StatusBadRequest)

	// 未登记回路的路灯不能用于按灯建立区域故障
	noCircuit, err := h.lamps.Create(ctx, lamp.CreateRequest{
		Code: "LD-M-004", RoadName: "无名路", LampType: lamp.LampTypeLED,
	})
	require.NoError(t, err)
	_, err = h.regions.Create(ctx, buildCreateReq([]uint{noCircuit.ID}, ""))
	requireStatus(t, err, http.StatusBadRequest)

	// 存在未闭环单灯故障的路灯不允许纳入
	_, err = h.faults.Create(ctx, fault.CreateRequest{
		LampID: same2.ID, FaultType: "灯不亮", Description: "单灯故障占用",
	})
	require.NoError(t, err)
	_, err = h.regions.Create(ctx, buildCreateReq([]uint{same1.ID, same2.ID}, ""))
	requireStatus(t, err, http.StatusConflict)

	// 按不存在回路建立被拒绝
	_, err = h.regions.Create(ctx, buildCreateReq(nil, "WL-NOT-EXIST"))
	requireStatus(t, err, http.StatusBadRequest)
}

func TestRegionCircuitOverview(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	l1 := h.createLampWithCircuit(t, "LD-O-001", "滨江路", "WL-BJ-01")
	l2 := h.createLampWithCircuit(t, "LD-O-002", "滨江路", "WL-BJ-01")
	l3 := h.createLampWithCircuit(t, "LD-O-003", "学院路", "WL-XY-01")

	// 回路一: 2 盏, 全部修复并闭环(故障发生在 5 小时前, 用于验证恢复耗时)
	closedReq := buildCreateReq([]uint{l1.ID, l2.ID}, "")
	closedReq.ReportedAt = nowMinus(5 * time.Hour)
	closed, err := h.regions.Create(ctx, closedReq)
	require.NoError(t, err)
	_, err = h.regions.Dispatch(ctx, closed.ID, region.DispatchRequest{Repairman: "陈鹏"})
	require.NoError(t, err)
	children, err := h.faults.Repository().ListByRegion(ctx, closed.ID)
	require.NoError(t, err)
	for _, child := range children {
		_, err = h.regions.ResolveLamp(ctx, closed.ID, child.ID, region.ResolveLampRequest{Result: repair.ResultFixed})
		require.NoError(t, err)
	}
	_, err = h.regions.Close(ctx, closed.ID, region.CloseRequest{})
	require.NoError(t, err)

	// 回路二: 1 盏, 派工后仍在处置(遗留)
	open, err := h.regions.Create(ctx, buildCreateReq([]uint{l3.ID}, ""))
	require.NoError(t, err)
	_, err = h.regions.Dispatch(ctx, open.ID, region.DispatchRequest{Repairman: "周涛"})
	require.NoError(t, err)

	overview, err := h.regions.CircuitOverview(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), overview.TotalRegions)
	require.Equal(t, int64(1), overview.OpenRegions)
	require.Equal(t, int64(3), overview.AffectedLamps)
	require.Equal(t, int64(2), overview.RestoredLamps)
	require.Equal(t, int64(1), overview.LegacyLamps)
	require.Len(t, overview.Circuits, 2)

	// 有在途区域单且有遗留的回路排在前面
	require.Equal(t, "WL-XY-01", overview.Circuits[0].CircuitCode)
	require.Equal(t, int64(1), overview.Circuits[0].LegacyTotal)
	require.True(t, overview.Circuits[0].HasActiveRegion)

	var closedStat *region.CircuitStat
	for i := range overview.Circuits {
		if overview.Circuits[i].CircuitCode == "WL-BJ-01" {
			closedStat = &overview.Circuits[i]
		}
	}
	require.NotNil(t, closedStat)
	require.Equal(t, int64(0), closedStat.LegacyTotal)
	require.Equal(t, int64(2), closedStat.RestoredTotal)
	// 故障发生于 5 小时前, 平均恢复耗时应接近 5 小时
	require.Greater(t, closedStat.AvgRecoveryHours, 4.0)
}

func TestRegionAddAndRemoveLampsBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	l1 := h.createLamp(t, "LD-A-001")
	l2 := h.createLamp(t, "LD-A-002")
	other := h.createLampWithCircuit(t, "LD-A-003", "中山路", "WL-OTHER-99")

	entity, err := h.regions.Create(ctx, buildCreateReq([]uint{l1.ID}, ""))
	require.NoError(t, err)

	// 追加同回路路灯
	require.NoError(t, h.regions.AddLamps(ctx, entity.ID, []uint{l2.ID, l2.ID}))
	updated, err := h.regions.Get(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, 2, updated.AffectedCount)

	// 跨回路追加被拒绝
	requireStatus(t, h.regions.AddLamps(ctx, entity.ID, []uint{other.ID}), http.StatusBadRequest)

	// 派工后不允许追加 / 移除
	_, err = h.regions.Dispatch(ctx, entity.ID, region.DispatchRequest{Repairman: "陈鹏"})
	require.NoError(t, err)
	children, err := h.faults.Repository().ListByRegion(ctx, entity.ID)
	require.NoError(t, err)
	requireStatus(t, h.regions.AddLamps(ctx, entity.ID, []uint{other.ID}), http.StatusConflict)
	requireStatus(t, h.regions.RemoveLamp(ctx, entity.ID, children[0].ID), http.StatusConflict)
}

func TestRegionChildFaultsBlockedFromSingleFaultAPI(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	l1 := h.createLamp(t, "LD-B-001")
	entity, err := h.regions.Create(ctx, buildCreateReq([]uint{l1.ID}, ""))
	require.NoError(t, err)
	children, err := h.faults.Repository().ListByRegion(ctx, entity.ID)
	require.NoError(t, err)
	child := children[0]

	// 子故障不允许在故障登记模块直接关闭
	_, err = h.faults.Close(ctx, child.ID, fault.CloseRequest{Remark: "提前关闭"})
	requireStatus(t, err, http.StatusConflict)

	// 子故障也不允许在维修录入模块单独开工, 必须由区域单统一派工
	_, err = h.repairs.Create(ctx, repair.CreateRequest{FaultID: child.ID, Repairman: "私自维修"})
	requireStatus(t, err, http.StatusConflict)
}
