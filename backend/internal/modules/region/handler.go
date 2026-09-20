package region

import (
	"github.com/gin-gonic/gin"

	"streetlight/internal/httpx"
	"streetlight/internal/response"
)

// Handler 处理区域故障相关的 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler 构造区域故障处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List 查询区域故障单列表。
func (h *Handler) List(c *gin.Context) {
	var query ListQuery
	if err := httpx.BindQuery(c, &query); err != nil {
		response.Fail(c, err)
		return
	}
	items, total, page, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.NewPageData(items, total, page.Page, page.PageSize))
}

// Create 建立区域故障并关联同回路多盏路灯。
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	entity, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, entity)
}

// Get 查询区域故障详情(含逐盏处置进展)。
func (h *Handler) Get(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	detail, err := h.service.Detail(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// AddLamps 派工前追加受影响路灯。
func (h *Handler) AddLamps(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req AddLampsRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	if err := h.service.AddLamps(c.Request.Context(), id, req.LampIDs); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": id})
}

// RemoveLamp 派工前移除误关联路灯。
func (h *Handler) RemoveLamp(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	faultID, err := httpx.ParseID(c, "faultId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := h.service.RemoveLamp(c.Request.Context(), id, faultID); err != nil {
		response.Fail(c, err)
		return
	}
	response.NoContent(c)
}

// Dispatch 统一派工。
func (h *Handler) Dispatch(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req DispatchRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	entity, err := h.service.Dispatch(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, entity)
}

// ResolveLamp 逐盏登记处置结果。
func (h *Handler) ResolveLamp(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	faultID, err := httpx.ParseID(c, "faultId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req ResolveLampRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	item, err := h.service.ResolveLamp(c.Request.Context(), id, faultID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// RedispatchLamp 对单盏未恢复路灯再次派工。
func (h *Handler) RedispatchLamp(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	faultID, err := httpx.ParseID(c, "faultId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req RedispatchRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	item, err := h.service.RedispatchLamp(c.Request.Context(), id, faultID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Close 区域故障整体闭环。
func (h *Handler) Close(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req CloseRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	entity, err := h.service.Close(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, entity)
}

// CircuitOverview 按回路汇总影响范围、恢复耗时与遗留数量。
func (h *Handler) CircuitOverview(c *gin.Context) {
	result, err := h.service.CircuitOverview(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Metadata 返回区域故障字典。
func (h *Handler) Metadata(c *gin.Context) {
	response.OK(c, h.service.Metadata())
}
