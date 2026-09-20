package areafault

import "time"

// 区域故障处置状态: 派工、完工与闭环都在区域故障上统一推进。
const (
	StatusPending    = "pending"    // 待派工
	StatusDispatched = "dispatched" // 已派工(处置中)
	StatusRestored   = "restored"   // 全部恢复(待闭环)
	StatusClosed     = "closed"     // 已闭环
)

// 区域故障诱因(线路或控制箱)。
const (
	CauseLine    = "line"    // 线路故障
	CauseCabinet = "cabinet" // 控制箱故障
)

// 逐盏处置结果。
const (
	ItemResultPending   = "pending"   // 待处置(初始态)
	ItemResultRecovered = "recovered" // 已恢复
	ItemResultParts     = "parts"     // 待配件
	ItemResultObserving = "observing" // 观察中
	ItemResultUnfixable = "unfixable" // 无法修复
)

// Statuses 返回全部区域故障状态取值。
func Statuses() []string {
	return []string{StatusPending, StatusDispatched, StatusRestored, StatusClosed}
}

// Causes 返回全部诱因取值。
func Causes() []string {
	return []string{CauseLine, CauseCabinet}
}

// ItemResults 返回逐盏处置结果取值(不含待处置, 待处置为初始态)。
func ItemResults() []string {
	return []string{ItemResultRecovered, ItemResultParts, ItemResultObserving, ItemResultUnfixable}
}

// IsValidStatus 校验区域故障状态取值。
func IsValidStatus(status string) bool {
	for _, item := range Statuses() {
		if item == status {
			return true
		}
	}
	return false
}

// IsValidCause 校验诱因取值。
func IsValidCause(cause string) bool {
	for _, item := range Causes() {
		if item == cause {
			return true
		}
	}
	return false
}

// IsValidItemResult 校验逐盏处置结果取值。
func IsValidItemResult(result string) bool {
	for _, item := range ItemResults() {
		if item == result {
			return true
		}
	}
	return false
}

// IsOpen 判断区域故障是否仍处于未闭环状态。
func IsOpen(status string) bool {
	return status == StatusPending || status == StatusDispatched || status == StatusRestored
}

// canTransitTo 校验区域故障状态流转是否合法。
// 待派工 -> 已派工 / 已闭环; 已派工 <-> 全部恢复; 全部恢复 -> 已闭环。
func canTransitTo(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusPending:
		return to == StatusDispatched || to == StatusClosed
	case StatusDispatched:
		return to == StatusRestored || to == StatusClosed || to == StatusPending
	case StatusRestored:
		return to == StatusClosed || to == StatusDispatched
	default:
		return false
	}
}

// AreaFault 区域故障主单: 线路或控制箱故障导致同一回路多盏路灯受影响时建立,
// 派工、完工与闭环在主单上统一推进, 逐盏处置结果登记在明细中。
type AreaFault struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	AreaNo          string     `gorm:"size:64;uniqueIndex;not null" json:"area_no"`
	Cause           string     `gorm:"size:32;index;not null" json:"cause"`
	CircuitName     string     `gorm:"size:128;index;not null" json:"circuit_name"`
	RoadName        string     `gorm:"size:128;index" json:"road_name"`
	FaultType       string     `gorm:"size:32;index;not null" json:"fault_type"`
	FaultLevel      string     `gorm:"size:32;index;not null;default:normal" json:"fault_level"`
	Description     string     `gorm:"size:512" json:"description"`
	Reporter        string     `gorm:"size:64" json:"reporter"`
	ReportedAt      time.Time  `gorm:"index;not null" json:"reported_at"`
	Status          string     `gorm:"size:32;index;not null;default:pending" json:"status"`
	TotalCount      int        `gorm:"not null;default:0" json:"total_count"`
	RecoveredCount  int        `gorm:"not null;default:0" json:"recovered_count"`
	PendingCount    int        `gorm:"not null;default:0" json:"pending_count"` // 遗留(未恢复)数量
	Assignee        string     `gorm:"size:64;index" json:"assignee"`
	RepairTeam      string     `gorm:"size:64;index" json:"repair_team"`
	ContactPhone    string     `gorm:"size:32" json:"contact_phone"`
	DispatchedAt    *time.Time `gorm:"index" json:"dispatched_at"`
	AllRestoredAt   *time.Time `json:"all_restored_at"`
	ClosedAt        *time.Time `json:"closed_at"`
	CloseRemark     string     `gorm:"size:255" json:"close_remark"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	// RestoreMinutes 仅用于响应展示的恢复耗时(分钟): 从上报到全部恢复, 不落库。
	RestoreMinutes *int64 `gorm:"-" json:"restore_minutes,omitempty"`
}

// TableName 指定表名。
func (AreaFault) TableName() string { return "area_fault" }

// AreaFaultItem 区域故障逐盏处置明细, 一条记录对应回路上一盏受影响路灯。
type AreaFaultItem struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	AreaFaultID  uint       `gorm:"index;not null" json:"area_fault_id"`
	LampID       uint       `gorm:"index;not null" json:"lamp_id"`
	LampCode     string     `gorm:"size:64;index" json:"lamp_code"`
	RoadName     string     `gorm:"size:128;index" json:"road_name"`
	FaultID      *uint      `gorm:"index" json:"fault_id"` // 关联的逐灯故障单
	Result       string     `gorm:"size:32;index;not null;default:pending" json:"result"`
	HandleRemark string     `gorm:"size:255" json:"handle_remark"`
	Handler      string     `gorm:"size:64" json:"handler"`
	HandledAt    *time.Time `json:"handled_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (AreaFaultItem) TableName() string { return "area_fault_item" }
