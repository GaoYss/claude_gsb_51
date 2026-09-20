package region

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"streetlight/internal/apperr"
	"streetlight/pkg/dbtx"
	"streetlight/pkg/pagination"
)

// Filter 是仓储层使用的区域故障查询条件。
type Filter struct {
	Keyword      string
	Status       string
	Cause        string
	RoadName     string
	RepairTeam   string
	ReportedFrom *time.Time
	ReportedTo   *time.Time
	OnlyOpen     bool
}

// Repository 负责区域故障单的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造区域故障仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) session(ctx context.Context) *gorm.DB {
	return dbtx.Session(ctx, r.db)
}

// DB 暴露底层连接, 供服务层开启跨模块事务。
func (r *Repository) DB() *gorm.DB { return r.db }

// Create 新增区域故障单。
func (r *Repository) Create(ctx context.Context, entity *RegionFault) error {
	if err := r.session(ctx).Create(entity).Error; err != nil {
		return fmt.Errorf("建立区域故障失败: %w", err)
	}
	return nil
}

// CreateWithUniqueNo 生成唯一区域单号并落库, 单号冲突时自动重试。
func (r *Repository) CreateWithUniqueNo(ctx context.Context, entity *RegionFault, prefix string) error {
	for attempt := 0; attempt < 5; attempt++ {
		sequence, err := r.NextSequence(ctx, prefix)
		if err != nil {
			return err
		}
		entity.No = fmt.Sprintf("%s%04d", prefix, sequence+attempt)
		err = r.Create(ctx, entity)
		if err == nil {
			return nil
		}
		if !isUniqueViolation(err) {
			return err
		}
	}
	return apperr.Conflict("区域单号生成冲突, 请稍后重试")
}

// NextSequence 返回指定前缀下可用的下一个流水号。
func (r *Repository) NextSequence(ctx context.Context, prefix string) (int, error) {
	var latest string
	err := r.session(ctx).Model(&RegionFault{}).
		Where("no LIKE ?", prefix+"%").
		Order("no DESC").
		Limit(1).
		Pluck("no", &latest).Error
	if err != nil {
		return 0, fmt.Errorf("生成区域单号失败: %w", err)
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

// Update 保存区域故障单全部字段。
func (r *Repository) Update(ctx context.Context, entity *RegionFault) error {
	if err := r.session(ctx).Save(entity).Error; err != nil {
		return fmt.Errorf("更新区域故障失败: %w", err)
	}
	return nil
}

// GetByID 按主键查询区域故障单。
func (r *Repository) GetByID(ctx context.Context, id uint) (*RegionFault, error) {
	var entity RegionFault
	err := r.session(ctx).First(&entity, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.NotFound("区域故障单不存在: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询区域故障失败: %w", err)
	}
	return &entity, nil
}

// List 分页查询区域故障单。
func (r *Repository) List(ctx context.Context, filter Filter, page pagination.Query) ([]RegionFault, int64, error) {
	base := func() *gorm.DB {
		return applyFilter(r.session(ctx).Model(&RegionFault{}), filter)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计区域故障总数失败: %w", err)
	}

	entities := make([]RegionFault, 0)
	if err := base().Order(page.OrderClause()).Offset(page.Offset()).Limit(page.Limit()).Find(&entities).Error; err != nil {
		return nil, 0, fmt.Errorf("查询区域故障列表失败: %w", err)
	}
	return entities, total, nil
}

// ListAll 查回全部区域故障单(用于按回路聚合, 数据量可控)。
func (r *Repository) ListAll(ctx context.Context) ([]RegionFault, error) {
	entities := make([]RegionFault, 0)
	err := r.session(ctx).Model(&RegionFault{}).Order("reported_at DESC, id DESC").Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("查询区域故障失败: %w", err)
	}
	return entities, nil
}

// applyFilter 统一拼装区域故障列表查询条件。
func applyFilter(statement *gorm.DB, filter Filter) *gorm.DB {
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		statement = statement.Where(
			"no LIKE ? OR circuit_code LIKE ? OR road_name LIKE ? OR description LIKE ?",
			like, like, like, like,
		)
	}
	if filter.Status != "" {
		statement = statement.Where("status = ?", filter.Status)
	}
	if filter.Cause != "" {
		statement = statement.Where("cause = ?", filter.Cause)
	}
	if value := strings.TrimSpace(filter.RoadName); value != "" {
		statement = statement.Where("road_name = ?", value)
	}
	if value := strings.TrimSpace(filter.RepairTeam); value != "" {
		statement = statement.Where("dispatch_team = ?", value)
	}
	if filter.ReportedFrom != nil {
		statement = statement.Where("reported_at >= ?", *filter.ReportedFrom)
	}
	if filter.ReportedTo != nil {
		statement = statement.Where("reported_at < ?", *filter.ReportedTo)
	}
	if filter.OnlyOpen {
		statement = statement.Where("status <> ?", StatusClosed)
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
