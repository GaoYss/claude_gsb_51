package region

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/lamp"
	"streetlight/internal/modules/repair"
)

// Module 区域故障处置模块: 线路或控制箱故障时, 按回路关联多盏路灯统一处置。
type Module struct {
	repository *Repository
	service    *Service
	handler    *Handler
}

// New 构造区域故障模块, 依赖路灯台账、故障登记与维修记录模块。
func New(db *gorm.DB, lamps *lamp.Repository, faults *fault.Service, repairs *repair.Service, repairRepo *repair.Repository) *Module {
	repository := NewRepository(db)
	service := NewService(repository, lamps, faults, repairs, repairRepo)
	return &Module{
		repository: repository,
		service:    service,
		handler:    NewHandler(service),
	}
}

// Name 实现 module.Module 接口。
func (m *Module) Name() string { return "区域故障处置" }

// Models 实现 module.Module 接口。
func (m *Module) Models() []any { return []any{&RegionFault{}} }

// RegisterRoutes 实现 module.Module 接口。
func (m *Module) RegisterRoutes(api *gin.RouterGroup) {
	group := api.Group("/region-faults")
	{
		group.GET("", m.handler.List)
		group.POST("", m.handler.Create)
		group.GET("/meta", m.handler.Metadata)
		group.GET("/circuits/overview", m.handler.CircuitOverview)
		group.GET("/:id", m.handler.Get)
		group.POST("/:id/lamps", m.handler.AddLamps)
		group.DELETE("/:id/lamps/:faultId", m.handler.RemoveLamp)
		group.POST("/:id/dispatch", m.handler.Dispatch)
		group.POST("/:id/close", m.handler.Close)
		group.POST("/:id/lamps/:faultId/resolve", m.handler.ResolveLamp)
		group.POST("/:id/lamps/:faultId/redispatch", m.handler.RedispatchLamp)
	}
}
