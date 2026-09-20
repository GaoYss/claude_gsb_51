package region

import (
	"time"

	"streetlight/pkg/pagination"
)

// CreateRequest 建立区域故障请求: 指定同一回路下受影响的多盏路灯(或整条回路)。
type CreateRequest struct {
	CircuitCode   string `json:"circuit_code" binding:"max=64"` // 指定回路时关联该回路全部路灯
	LampIDs       []uint `json:"lamp_ids"`                      // 也可显式指定受影响路灯, 必须属于同一回路
	Cause         string `json:"cause" binding:"required,max=32"`
	FaultLevel    string `json:"fault_level" binding:"omitempty,oneof=low normal high urgent"`
	Source        string `json:"source" binding:"omitempty,oneof=inspection citizen monitoring other"`
	Description   string `json:"description" binding:"required,max=512"`
	Reporter      string `json:"reporter" binding:"max=64"`
	ReporterPhone string `json:"reporter_phone" binding:"max=32"`
	ReportedAt    string `json:"reported_at" binding:"omitempty,max=32"`
}

// AddLampsRequest 在派工前向区域故障单追加受影响路灯。
type AddLampsRequest struct {
	LampIDs []uint `json:"lamp_ids" binding:"required,min=1"`
}

// DispatchRequest 统一派工请求, 一次派工对区域单内全部受影响路灯生效。
type DispatchRequest struct {
	Repairman    string `json:"repairman" binding:"required,max=64"`
	RepairTeam   string `json:"repair_team" binding:"max=64"`
	ContactPhone string `json:"contact_phone" binding:"max=32"`
	StartedAt    string `json:"started_at" binding:"omitempty,max=32"`
	Content      string `json:"content" binding:"max=512"`
}

// RedispatchRequest 对单盏处置未恢复的路灯再次派工(返修)。
type RedispatchRequest struct {
	Repairman    string `json:"repairman" binding:"required,max=64"`
	RepairTeam   string `json:"repair_team" binding:"max=64"`
	ContactPhone string `json:"contact_phone" binding:"max=32"`
	StartedAt    string `json:"started_at" binding:"omitempty,max=32"`
	Content      string `json:"content" binding:"max=512"`
}

// ResolveLampRequest 逐盏登记处置结果: 完工一盏、登记一盏。
type ResolveLampRequest struct {
	FinishedAt string   `json:"finished_at" binding:"omitempty,max=32"`
	Result     string   `json:"result" binding:"required,oneof=fixed pending_parts observing unfixable"`
	Content    string   `json:"content" binding:"omitempty,max=512"`
	Materials  string   `json:"materials" binding:"omitempty,max=255"`
	Cost       *float64 `json:"cost" binding:"omitempty,min=0"`
	Remark     string   `json:"remark" binding:"omitempty,max=255"`
}

// CloseRequest 区域故障整体闭环请求。
type CloseRequest struct {
	Remark string `json:"remark" binding:"max=255"`
}

// ListQuery 区域故障列表查询条件。
type ListQuery struct {
	pagination.Params
	Keyword    string `form:"keyword"` // 区域单号 / 回路编号 / 道路 / 描述
	Status     string `form:"status"`
	Cause      string `form:"cause"`
	RoadName   string `form:"road_name"`
	RepairTeam string `form:"repair_team"`
	StartDate  string `form:"start_date"`
	EndDate    string `form:"end_date"`
	OnlyOpen   bool   `form:"only_open"`
}

// RegionLampItem 区域故障单内一盏受影响路灯的处置进展。
type RegionLampItem struct {
	FaultID      uint       `json:"fault_id"`
	FaultNo      string     `json:"fault_no"`
	FaultStatus  string     `json:"fault_status"`
	LampID       uint       `json:"lamp_id"`
	LampCode     string     `json:"lamp_code"`
	LampName     string     `json:"lamp_name"`
	RoadName     string     `json:"road_name"`
	RunStatus    string     `json:"run_status"`
	RepairID     *uint      `json:"repair_id"`
	RepairNo     string     `json:"repair_no"`
	Repairman    string     `json:"repairman"`
	RepairStatus string     `json:"repair_status"`
	RepairResult string     `json:"repair_result"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Restored     bool       `json:"restored"` // 该盏路灯是否已恢复(已修复/已关闭)
}

// Detail 区域故障详情: 主单 + 每盏路灯的逐盏处置进展。
type Detail struct {
	*RegionFault
	Items []RegionLampItem `json:"items"`
}

// CircuitStat 按回路汇总的区域故障概览一行。
type CircuitStat struct {
	CircuitCode      string    `json:"circuit_code"`
	RoadNames        []string  `json:"road_names"`
	RegionTotal      int64     `json:"region_total"`       // 区域故障单数
	OpenTotal        int64     `json:"open_total"`         // 未闭环区域单数
	AffectedTotal    int64     `json:"affected_total"`     // 累计影响路灯数
	RestoredTotal    int64     `json:"restored_total"`     // 已恢复路灯数
	LegacyTotal      int64     `json:"legacy_total"`       // 遗留(未恢复)路灯数
	AvgRecoveryHours float64   `json:"avg_recovery_hours"` // 平均恢复耗时(全部恢复的区域单）
	LastReportedAt   time.Time `json:"last_reported_at"`
	HasActiveRegion  bool      `json:"has_active_region"`
}

// CircuitOverview 按回路给出影响范围、恢复耗时与遗留数量。
type CircuitOverview struct {
	TotalRegions  int64         `json:"total_regions"`
	OpenRegions   int64         `json:"open_regions"`
	AffectedLamps int64         `json:"affected_lamps"`
	RestoredLamps int64         `json:"restored_lamps"`
	LegacyLamps   int64         `json:"legacy_lamps"`
	Circuits      []CircuitStat `json:"circuits"`
}

// Meta 区域故障模块字典。
type Meta struct {
	Statuses []string `json:"statuses"`
	Causes   []string `json:"causes"`
	Levels   []string `json:"levels"`
	Sources  []string `json:"sources"`
}
