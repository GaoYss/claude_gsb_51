package areafault

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"streetlight/internal/apperr"
	"streetlight/pkg/pagination"
)

// openStatuses 是未闭环区域故障的状态集合。
var openStatuses = []string{StatusPending, StatusDispatched, StatusRestored}

// Filter 是仓储层使用的区域故障查询条件。
type Filter struct {
	Keyword     string
	Status      string
	Cause       string
	FaultType   string
	FaultLevel  string
	CircuitName string
	RoadName    string
	OnlyOpen    bool
}

// Repository 负责区域故障的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造区域故障仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// DB 暴露底层连接, 供需要跨表事务的服务层使用。
func (r *Repository) DB() *gorm.DB { return r.db }

// Create 新增区域故障主单。
func (r *Repository) Create(ctx context.Context, entity *AreaFault) error {
	if err := r.session(ctx).Create(entity).Error; err != nil {
		return fmt.Errorf("登记区域故障失败: %w", err)
	}
	return nil
}

// CreateWithUniqueNo 生成唯一区域单号并落库, 单号冲突时自动重试。
func (r *Repository) CreateWithUniqueNo(ctx context.Context, entity *AreaFault, prefix string) error {
	for attempt := 0; attempt < 5; attempt++ {
		sequence, err := r.NextSequence(ctx, prefix)
		if err != nil {
			return err
		}
		entity.AreaNo = fmt.Sprintf("%s%04d", prefix, sequence+attempt)
		err = r.Create(ctx, entity)
		if err == nil {
			return nil
		}
		if !isUniqueViolation(err) {
			return err
		}
	}
	return apperr.Conflict("区域故障单号生成冲突, 请稍后重试")
}

// NextSequence 返回指定前缀下可用的下一个流水号。
func (r *Repository) NextSequence(ctx context.Context, prefix string) (int, error) {
	var latest string
	err := r.session(ctx).Model(&AreaFault{}).
		Where("area_no LIKE ?", prefix+"%").
		Order("area_no DESC").
		Limit(1).
		Pluck("area_no", &latest).Error
	if err != nil {
		return 0, fmt.Errorf("生成区域故障单号失败: %w", err)
	}
	if latest == "" {
		return 1, nil
	}
	value, convErr := strconv.Atoi(strings.TrimPrefix(latest, prefix))
	if convErr != nil {
		return 1, nil
	}
	return value + 1, nil
}

// Update 保存区域故障主单全部字段。
func (r *Repository) Update(ctx context.Context, entity *AreaFault) error {
	if err := r.session(ctx).Save(entity).Error; err != nil {
		return fmt.Errorf("更新区域故障失败: %w", err)
	}
	return nil
}

// GetByID 按主键查询区域故障。
func (r *Repository) GetByID(ctx context.Context, id uint) (*AreaFault, error) {
	var entity AreaFault
	err := r.session(ctx).First(&entity, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.NotFound("区域故障不存在: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询区域故障失败: %w", err)
	}
	return &entity, nil
}

// List 分页查询区域故障。
func (r *Repository) List(ctx context.Context, filter Filter, page pagination.Query) ([]AreaFault, int64, error) {
	base := func() *gorm.DB {
		return applyFilter(r.session(ctx).Model(&AreaFault{}), filter)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计区域故障总数失败: %w", err)
	}

	entities := make([]AreaFault, 0)
	if err := base().Order(page.OrderClause()).Offset(page.Offset()).Limit(page.Limit()).Find(&entities).Error; err != nil {
		return nil, 0, fmt.Errorf("查询区域故障列表失败: %w", err)
	}
	return entities, total, nil
}

// CreateItems 批量新增逐盏明细。
func (r *Repository) CreateItems(ctx context.Context, items []AreaFaultItem) error {
	if len(items) == 0 {
		return nil
	}
	if err := r.session(ctx).Create(&items).Error; err != nil {
		return fmt.Errorf("写入区域故障明细失败: %w", err)
	}
	return nil
}

// ListItems 查询某条区域故障的全部逐盏明细。
func (r *Repository) ListItems(ctx context.Context, areaFaultID uint) ([]AreaFaultItem, error) {
	items := make([]AreaFaultItem, 0)
	err := r.session(ctx).
		Where("area_fault_id = ?", areaFaultID).
		Order("id ASC").
		Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("查询区域故障明细失败: %w", err)
	}
	return items, nil
}

// GetItem 按主键查询明细。
func (r *Repository) GetItem(ctx context.Context, id uint) (*AreaFaultItem, error) {
	var item AreaFaultItem
	err := r.session(ctx).First(&item, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.NotFound("区域故障处置明细不存在: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询区域故障明细失败: %w", err)
	}
	return &item, nil
}

// UpdateItem 保存明细全部字段。
func (r *Repository) UpdateItem(ctx context.Context, item *AreaFaultItem) error {
	if err := r.session(ctx).Save(item).Error; err != nil {
		return fmt.Errorf("更新区域故障明细失败: %w", err)
	}
	return nil
}

// CountItemsByResult 统计某条区域故障各处置结果的明细数量。
func (r *Repository) CountItemsByResult(ctx context.Context, areaFaultID uint) (map[string]int64, error) {
	type row struct {
		Label string
		Total int64
	}
	rows := make([]row, 0)
	err := r.session(ctx).Model(&AreaFaultItem{}).
		Select("result AS label, COUNT(*) AS total").
		Where("area_fault_id = ?", areaFaultID).
		Group("result").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("统计区域故障处置结果失败: %w", err)
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Label] = item.Total
	}
	return result, nil
}

// CountOpenByLamp 统计某盏路灯关联的未闭环区域故障数量, 实现路灯模块的删除校验端口。
func (r *Repository) CountOpenByLamp(ctx context.Context, lampID uint) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&AreaFaultItem{}).
		Distinct("area_fault_item.area_fault_id").
		Joins("JOIN area_fault ON area_fault.id = area_fault_item.area_fault_id").
		Where("area_fault_item.lamp_id = ? AND area_fault.status IN ?", lampID, openStatuses).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计路灯未闭环区域故障失败: %w", err)
	}
	return count, nil
}

// CircuitStatRow 是按回路聚合的概览行(仓储层)。
type CircuitStatRow struct {
	CircuitName    string
	RoadName       string
	OpenTotal      int64
	ClosedTotal    int64
	AffectedLamps  int64
	RecoveredLamps int64
	PendingLamps   int64
}

// CircuitStats 按回路聚合区域故障的影响范围(受影响/已恢复/遗留灯数)与单量。
func (r *Repository) CircuitStats(ctx context.Context) ([]CircuitStatRow, error) {
	rows := make([]CircuitStatRow, 0)
	err := r.session(ctx).Model(&AreaFault{}).
		Select(`
			circuit_name AS circuit_name,
			MIN(road_name) AS road_name,
			SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END) AS open_total,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS closed_total,
			SUM(total_count) AS affected_lamps,
			SUM(recovered_count) AS recovered_lamps,
			SUM(pending_count) AS pending_lamps
		`, openStatuses, StatusClosed).
		Group("circuit_name").
		Order("open_total DESC, pending_lamps DESC, circuit_name ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("按回路统计区域故障失败: %w", err)
	}
	return rows, nil
}

// RestoreDurationByCircuit 取已全部恢复主单的上报与恢复时刻, 由应用层计算平均恢复耗时,
// 规避 sqlite(julianday) 与 postgres(EXTRACT) 的时间差方言差异。
func (r *Repository) RestoreDurationByCircuit(ctx context.Context) (map[string][]RestoreDuration, error) {
	rows := make([]RestoreDuration, 0)
	err := r.session(ctx).Model(&AreaFault{}).
		Select("circuit_name, reported_at, all_restored_at").
		Where("all_restored_at IS NOT NULL").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询区域故障恢复耗时失败: %w", err)
	}
	result := make(map[string][]RestoreDuration, len(rows))
	for _, row := range rows {
		result[row.CircuitName] = append(result[row.CircuitName], row)
	}
	return result, nil
}

// RestoreDuration 是计算恢复耗时所需的一对时间点。
type RestoreDuration struct {
	CircuitName   string
	ReportedAt    time.Time
	AllRestoredAt *time.Time
}

// GroupByCauseAndStatus 分别按诱因、状态统计区域故障数量。
func (r *Repository) GroupByCauseAndStatus(ctx context.Context) (map[string]int64, map[string]int64, error) {
	countBy := func(column string) (map[string]int64, error) {
		type row struct {
			Label string
			Total int64
		}
		rows := make([]row, 0)
		if err := r.session(ctx).Model(&AreaFault{}).
			Select(column + " AS label, COUNT(*) AS total").
			Group(column).
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("按 %s 统计区域故障失败: %w", column, err)
		}
		result := make(map[string]int64, len(rows))
		for _, item := range rows {
			result[item.Label] = item.Total
		}
		return result, nil
	}

	causeCounts, err := countBy("cause")
	if err != nil {
		return nil, nil, err
	}
	statusCounts, err := countBy("status")
	if err != nil {
		return nil, nil, err
	}
	return causeCounts, statusCounts, nil
}

// applyFilter 统一拼装区域故障列表查询条件。
func applyFilter(statement *gorm.DB, filter Filter) *gorm.DB {
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		statement = statement.Where(
			"area_no LIKE ? OR circuit_name LIKE ? OR road_name LIKE ? OR description LIKE ?",
			like, like, like, like,
		)
	}
	if filter.Status != "" {
		statement = statement.Where("status = ?", filter.Status)
	}
	if filter.Cause != "" {
		statement = statement.Where("cause = ?", filter.Cause)
	}
	if filter.FaultType != "" {
		statement = statement.Where("fault_type = ?", filter.FaultType)
	}
	if filter.FaultLevel != "" {
		statement = statement.Where("fault_level = ?", filter.FaultLevel)
	}
	if value := strings.TrimSpace(filter.CircuitName); value != "" {
		statement = statement.Where("circuit_name = ?", value)
	}
	if value := strings.TrimSpace(filter.RoadName); value != "" {
		statement = statement.Where("road_name = ?", value)
	}
	if filter.OnlyOpen {
		statement = statement.Where("status IN ?", openStatuses)
	}
	return statement
}

// isUniqueViolation 兼容 sqlite 与 postgres 的唯一约束冲突判断。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "unique violation")
}
