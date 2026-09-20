package areafault

import "streetlight/pkg/pagination"

// CreateRequest 建立区域故障: 同一回路受影响的多盏路灯一次性关联。
type CreateRequest struct {
	Cause       string `json:"cause" binding:"required,oneof=line cabinet"`
	CircuitName string `json:"circuit_name" binding:"required,max=128"`
	RoadName    string `json:"road_name" binding:"max=128"`
	LampIDs     []uint `json:"lamp_ids" binding:"required,min=1,max=200,dive,gt=0"`
	FaultType   string `json:"fault_type" binding:"required,max=32"`
	FaultLevel  string `json:"fault_level" binding:"omitempty,oneof=low normal high urgent"`
	Description string `json:"description" binding:"required,max=512"`
	Reporter    string `json:"reporter" binding:"max=64"`
	ReportedAt  string `json:"reported_at" binding:"omitempty,max=32"`
}

// DispatchRequest 派工: 在区域故障上统一指派班组与负责人。
type DispatchRequest struct {
	Assignee     string `json:"assignee" binding:"required,max=64"`
	RepairTeam   string `json:"repair_team" binding:"max=64"`
	ContactPhone string `json:"contact_phone" binding:"max=32"`
}

// HandleItemRequest 逐盏登记处置结果。
type HandleItemRequest struct {
	Result string `json:"result" binding:"required,oneof=recovered parts observing unfixable"`
	Remark string `json:"remark" binding:"max=255"`
	Handler string `json:"handler" binding:"max=64"`
}

// CloseRequest 区域故障整体闭环。
type CloseRequest struct {
	Remark string `json:"remark" binding:"max=255"`
}

// ListQuery 区域故障列表查询条件。
type ListQuery struct {
	pagination.Params
	Keyword     string `form:"keyword"` // 区域单号 / 回路 / 道路 / 描述
	Status      string `form:"status"`
	Cause       string `form:"cause"`
	FaultType   string `form:"fault_type"`
	FaultLevel  string `form:"fault_level"`
	CircuitName string `form:"circuit_name"`
	RoadName    string `form:"road_name"`
	OnlyOpen    bool   `form:"only_open"`
}

// Meta 区域故障模块字典。
type Meta struct {
	Statuses         []string `json:"statuses"`
	Causes           []string `json:"causes"`
	ItemResults      []string `json:"item_results"`
	FaultTypes       []string `json:"fault_types"`
	Levels           []string `json:"levels"`
}

// AreaFaultDetail 区域故障详情: 主单 + 逐盏明细。
type AreaFaultDetail struct {
	*AreaFault
	Items []AreaFaultItem `json:"items"`
}
