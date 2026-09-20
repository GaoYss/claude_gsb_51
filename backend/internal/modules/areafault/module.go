package areafault

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module 区域故障模块: 线路或控制箱故障导致同一回路多盏路灯受影响时,
// 建立区域故障主单统一派工、逐盏登记处置结果, 全部恢复后才允许整体闭环。
type Module struct {
	repository *Repository
	service    *Service
	handler    *Handler
}

// New 构造区域故障模块, lamps/faults 分别为路灯台账与故障登记模块提供的端口。
func New(db *gorm.DB, lamps LampPort, faults FaultPort) *Module {
	repository := NewRepository(db)
	service := NewService(repository, lamps, faults)
	return &Module{
		repository: repository,
		service:    service,
		handler:    NewHandler(service),
	}
}

// Service 暴露业务服务, 供测试与其它模块装配使用。
func (m *Module) Service() *Service { return m.service }

// Repository 暴露仓储, 供路灯模块装配未闭环区域故障计数器。
func (m *Module) Repository() *Repository { return m.repository }

// Name 实现 module.Module 接口。
func (m *Module) Name() string { return "区域故障" }

// Models 实现 module.Module 接口。
func (m *Module) Models() []any { return []any{&AreaFault{}, &AreaFaultItem{}} }

// RegisterRoutes 实现 module.Module 接口。
func (m *Module) RegisterRoutes(api *gin.RouterGroup) {
	group := api.Group("/area-faults")
	{
		group.GET("", m.handler.List)
		group.POST("", m.handler.Create)
		group.GET("/meta", m.handler.Metadata)
		group.GET("/overview", m.handler.Overview)
		group.GET("/:id", m.handler.Get)
		group.POST("/:id/dispatch", m.handler.Dispatch)
		group.POST("/:id/close", m.handler.Close)
		group.POST("/items/:itemId/handle", m.handler.HandleItem)
	}
}
