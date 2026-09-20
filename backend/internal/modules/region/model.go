package region

import "time"

// 区域故障单状态。
const (
	StatusPending    = "pending"    // 待派工
	StatusProcessing = "processing" // 处置中
	StatusClosed     = "closed"     // 已闭环
)

// 区域故障成因, 对应线路或控制箱故障。
const (
	CauseLine    = "线路故障"
	CauseCabinet = "控制箱故障"
)

// Statuses 返回全部区域故障状态取值。
func Statuses() []string {
	return []string{StatusPending, StatusProcessing, StatusClosed}
}

// Causes 返回区域故障允许的成因类型。
func Causes() []string {
	return []string{CauseLine, CauseCabinet}
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

// IsValidCause 校验区域故障成因取值。
func IsValidCause(cause string) bool {
	for _, item := range Causes() {
		if item == cause {
			return true
		}
	}
	return false
}

// RegionFault 区域故障单: 线路或控制箱故障时, 一条区域单关联同一回路下受影响的多盏路灯。
// 派工、完工与闭环都在区域单上统一推进, 每盏路灯的处置结果逐盏登记在子故障/维修记录上。
type RegionFault struct {
	ID uint   `gorm:"primaryKey" json:"id"`
	No string `gorm:"size:64;uniqueIndex;not null" json:"region_no"`
	// CircuitCode 受影响的照明回路编号, 区域单内的路灯必须属于同一回路。
	CircuitCode string `gorm:"size:64;index;not null" json:"circuit_code"`
	RoadName    string `gorm:"size:128;index" json:"road_name"`

	Cause         string `gorm:"size:32;index;not null" json:"cause"` // 线路故障 / 控制箱故障
	FaultLevel    string `gorm:"size:32;index;not null;default:normal" json:"fault_level"`
	Source        string `gorm:"size:32;index" json:"source"`
	Description   string `gorm:"size:512;not null" json:"description"`
	Reporter      string `gorm:"size:64" json:"reporter"`
	ReporterPhone string `gorm:"size:32" json:"reporter_phone"`

	ReportedAt    time.Time `gorm:"index;not null" json:"reported_at"`
	Status        string    `gorm:"size:32;index;not null;default:pending" json:"status"`
	AffectedCount int       `gorm:"not null;default:0" json:"affected_count"` // 受影响路灯数
	RestoredCount int       `gorm:"not null;default:0" json:"restored_count"` // 已恢复(子故障已修复或关闭)路灯数

	// 统一派工信息, 派工时写入; 派工对全部受影响路灯生效。
	DispatchedAt      *time.Time `gorm:"index" json:"dispatched_at"`
	DispatchRepairman string     `gorm:"size:64" json:"dispatch_repairman"`
	DispatchTeam      string     `gorm:"size:64;index" json:"dispatch_team"`
	ContactPhone      string     `gorm:"size:32" json:"contact_phone"`
	// RestoredAt 全部路灯恢复(逐盏登记为已修复)的时间, 用于统计恢复耗时。
	RestoredAt *time.Time `json:"restored_at"`

	ClosedAt    *time.Time `json:"closed_at"`
	CloseRemark string     `gorm:"size:255" json:"close_remark"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (RegionFault) TableName() string { return "region_fault" }

// IsOpen 判断区域故障是否尚未闭环。
func (r *RegionFault) IsOpen() bool {
	return r.Status != StatusClosed
}

// LegacyCount 遗留数量: 尚未恢复的路灯数。
func (r *RegionFault) LegacyCount() int {
	return r.AffectedCount - r.RestoredCount
}
