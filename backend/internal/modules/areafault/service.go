package areafault

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/lamp"
	"streetlight/pkg/pagination"
)

// areaSortSpec 定义区域故障列表允许的排序字段白名单。
var areaSortSpec = pagination.SortSpec{
	Allowed: map[string]string{
		"area_no":      "area_no",
		"reported_at":  "reported_at",
		"status":       "status",
		"cause":        "cause",
		"circuit_name": "circuit_name",
		"created_at":   "created_at",
		"updated_at":   "updated_at",
	},
	Default: "reported_at",
}

// LampPort 由路灯台账模块实现。
type LampPort interface {
	ListByIDs(ctx context.Context, ids []uint) ([]lamp.Lamp, error)
	UpdateRunStatus(ctx context.Context, id uint, status string) error
}

// FaultPort 由故障登记模块实现, 区域故障通过它为受影响路灯批量建单与兜底闭环。
type FaultPort interface {
	CountOpenByLamps(ctx context.Context, lampIDs []uint) (map[uint]int64, error)
	CreateForArea(ctx context.Context, lampID uint, faultType string, level string, description string, reporter string, reportedAt time.Time) (*fault.Fault, error)
	CloseIfOpen(ctx context.Context, id uint, remark string) (bool, error)
}

// Service 承载区域故障的业务规则: 建单关联、统一派工、逐盏处置与整体闭环。
type Service struct {
	repo   *Repository
	lamps  LampPort
	faults FaultPort
}

// NewService 构造区域故障服务。
func NewService(repo *Repository, lamps LampPort, faults FaultPort) *Service {
	return &Service{repo: repo, lamps: lamps, faults: faults}
}

// Create 建立区域故障: 校验同一回路的受影响路灯后, 为每盏路灯登记一条故障, 再落主单与逐盏明细。
func (s *Service) Create(ctx context.Context, req CreateRequest) (*AreaFaultDetail, error) {
	cause := strings.TrimSpace(req.Cause)
	if !IsValidCause(cause) {
		return nil, apperr.BadRequest("非法的区域故障诱因: %s", cause)
	}
	circuitName := strings.TrimSpace(req.CircuitName)
	if circuitName == "" {
		return nil, apperr.BadRequest("回路名称不能为空")
	}
	faultType := strings.TrimSpace(req.FaultType)
	if !isValidFaultType(faultType) {
		return nil, apperr.BadRequest("非法的故障类型: %s", faultType)
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		return nil, apperr.BadRequest("故障描述不能为空")
	}
	level := strings.TrimSpace(req.FaultLevel)
	if level == "" {
		level = fault.LevelNormal
	}
	reportedAt, err := parseReportedAt(req.ReportedAt)
	if err != nil {
		return nil, err
	}

	lampIDs := dedupPositive(req.LampIDs)
	if len(lampIDs) == 0 {
		return nil, apperr.BadRequest("至少关联一盏受影响路灯")
	}

	devices, err := s.lamps.ListByIDs(ctx, lampIDs)
	if err != nil {
		return nil, err
	}
	if len(devices) != len(lampIDs) {
		missing := findMissingLampIDs(lampIDs, devices)
		return nil, apperr.BadRequest("部分路灯不存在: %v", missing)
	}

	openCounts, err := s.faults.CountOpenByLamps(ctx, lampIDs)
	if err != nil {
		return nil, err
	}
	for lampID, count := range openCounts {
		if count > 0 {
			return nil, apperr.Conflict("路灯 %d 已存在 %d 条未闭环故障, 不能重复关联区域故障", lampID, count)
		}
	}

	roadName := strings.TrimSpace(req.RoadName)
	if roadName == "" {
		roadName = devices[0].RoadName
	}
	reporter := strings.TrimSpace(req.Reporter)
	if reporter == "" {
		reporter = "监控中心"
	}

	// 先逐盏登记故障(批量校验已排除冲突), 再落区域主单与明细。
	faultByLamp := make(map[uint]*fault.Fault, len(devices))
	for _, device := range devices {
		desc := description + "(" + CauseLabel(cause) + ", 回路: " + circuitName + ")"
		entity, createErr := s.faults.CreateForArea(ctx, device.ID, faultType, level, desc, reporter, reportedAt)
		if createErr != nil {
			return nil, createErr
		}
		faultByLamp[device.ID] = entity
	}

	area := &AreaFault{
		Cause:          cause,
		CircuitName:    circuitName,
		RoadName:       roadName,
		FaultType:      faultType,
		FaultLevel:     level,
		Description:    description,
		Reporter:       reporter,
		ReportedAt:     reportedAt,
		Status:         StatusPending,
		TotalCount:     len(devices),
		PendingCount:   len(devices),
	}
	if err := s.repo.CreateWithUniqueNo(ctx, area, areaNoPrefix(reportedAt)); err != nil {
		return nil, err
	}

	items := make([]AreaFaultItem, 0, len(devices))
	for _, device := range devices {
		linked := faultByLamp[device.ID]
		items = append(items, AreaFaultItem{
			AreaFaultID: area.ID,
			LampID:      device.ID,
			LampCode:    device.Code,
			RoadName:    device.RoadName,
			FaultID:     &linked.ID,
			Result:      ItemResultPending,
		})
	}
	if err := s.repo.CreateItems(ctx, items); err != nil {
		return nil, err
	}

	return s.buildDetail(ctx, area)
}

// List 分页查询区域故障。
func (s *Service) List(ctx context.Context, query ListQuery) ([]AreaFault, int64, pagination.Query, error) {
	page := pagination.Parse(query.Params, areaSortSpec)
	filter := Filter{
		Keyword:     strings.TrimSpace(query.Keyword),
		Status:      strings.TrimSpace(query.Status),
		Cause:       strings.TrimSpace(query.Cause),
		FaultType:   strings.TrimSpace(query.FaultType),
		FaultLevel:  strings.TrimSpace(query.FaultLevel),
		CircuitName: strings.TrimSpace(query.CircuitName),
		RoadName:    strings.TrimSpace(query.RoadName),
		OnlyOpen:    query.OnlyOpen,
	}
	if filter.Status != "" && !IsValidStatus(filter.Status) {
		return nil, 0, page, apperr.BadRequest("非法的区域故障状态: %s", filter.Status)
	}
	if filter.Cause != "" && !IsValidCause(filter.Cause) {
		return nil, 0, page, apperr.BadRequest("非法的区域故障诱因: %s", filter.Cause)
	}
	items, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, 0, page, err
	}
	for index := range items {
		items[index].FillRestoreDuration()
	}
	return items, total, page, nil
}

// Get 查询区域故障详情(含逐盏明细)。
func (s *Service) Get(ctx context.Context, id uint) (*AreaFaultDetail, error) {
	area, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.buildDetail(ctx, area)
}

// Dispatch 统一派工: 待派工 -> 处置中。
func (s *Service) Dispatch(ctx context.Context, id uint, req DispatchRequest) (*AreaFaultDetail, error) {
	area, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if area.Status == StatusClosed {
		return nil, apperr.Conflict("区域故障 %s 已闭环, 不允许派工", area.AreaNo)
	}
	if area.Status != StatusPending {
		return nil, apperr.Conflict("区域故障 %s 已派工, 无需重复派工", area.AreaNo)
	}
	assignee := strings.TrimSpace(req.Assignee)
	if assignee == "" {
		return nil, apperr.BadRequest("派工负责人不能为空")
	}

	area.Assignee = assignee
	area.RepairTeam = strings.TrimSpace(req.RepairTeam)
	area.ContactPhone = strings.TrimSpace(req.ContactPhone)
	now := time.Now()
	area.DispatchedAt = &now
	area.Status = StatusDispatched
	if err := s.repo.Update(ctx, area); err != nil {
		return nil, err
	}
	return s.buildDetail(ctx, area)
}

// HandleItem 逐盏登记处置结果, 并重算主单恢复进度与状态。
func (s *Service) HandleItem(ctx context.Context, itemID uint, req HandleItemRequest) (*AreaFaultDetail, error) {
	item, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	area, err := s.repo.GetByID(ctx, item.AreaFaultID)
	if err != nil {
		return nil, err
	}
	if area.Status == StatusPending {
		return nil, apperr.Conflict("区域故障 %s 尚未派工, 请先派工再登记处置结果", area.AreaNo)
	}
	if area.Status == StatusClosed {
		return nil, apperr.Conflict("区域故障 %s 已闭环, 不允许修改处置结果", area.AreaNo)
	}

	result := strings.TrimSpace(req.Result)
	if !IsValidItemResult(result) {
		return nil, apperr.BadRequest("非法的逐盏处置结果: %s", result)
	}
	handler := strings.TrimSpace(req.Handler)
	if handler == "" {
		handler = area.Assignee
	}

	wasRecovered := item.Result == ItemResultRecovered
	item.Result = result
	item.Handler = handler
	item.HandleRemark = strings.TrimSpace(req.Remark)
	now := time.Now()
	item.HandledAt = &now
	if err := s.repo.UpdateItem(ctx, item); err != nil {
		return nil, err
	}

	// 已恢复: 兜底闭环该灯故障并联动路灯状态; 由恢复改为其它结果时不重开故障, 由主单遗留数量体现。
	if result == ItemResultRecovered && !wasRecovered && item.FaultID != nil {
		if _, err := s.faults.CloseIfOpen(ctx, *item.FaultID, "区域故障 "+area.AreaNo+" 逐盏复核已恢复"); err != nil {
			slog.Warn("区域故障逐盏恢复后闭环关联故障失败", "area_no", area.AreaNo, "fault_id", *item.FaultID, "error", err)
		}
	}

	if err := s.refreshProgress(ctx, area); err != nil {
		return nil, err
	}
	return s.buildDetail(ctx, area)
}

// Close 整体闭环: 仅全部恢复后允许, 仍有遗留路灯时拒绝。
func (s *Service) Close(ctx context.Context, id uint, req CloseRequest) (*AreaFaultDetail, error) {
	area, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if area.Status == StatusClosed {
		return nil, apperr.Conflict("区域故障 %s 已闭环, 无需重复操作", area.AreaNo)
	}
	if area.PendingCount > 0 || area.Status != StatusRestored {
		return nil, apperr.Conflict("区域故障 %s 仍有 %d 盏路灯未恢复, 未全部恢复前不允许整体闭环", area.AreaNo, area.PendingCount)
	}

	now := time.Now()
	area.Status = StatusClosed
	area.ClosedAt = &now
	area.CloseRemark = strings.TrimSpace(req.Remark)
	if err := s.repo.Update(ctx, area); err != nil {
		return nil, err
	}
	return s.buildDetail(ctx, area)
}

// Metadata 返回区域故障模块字典。
func (s *Service) Metadata() *Meta {
	return &Meta{
		Statuses:    Statuses(),
		Causes:      Causes(),
		ItemResults: ItemResults(),
		FaultTypes:  fault.FaultTypes(),
		Levels:      fault.Levels(),
	}
}

// refreshProgress 依据明细重算主单的恢复/遗留数量并推进状态。
func (s *Service) refreshProgress(ctx context.Context, area *AreaFault) error {
	counts, err := s.repo.CountItemsByResult(ctx, area.ID)
	if err != nil {
		return err
	}
	recovered := int(counts[ItemResultRecovered])
	total := area.TotalCount
	pending := total - recovered
	if pending < 0 {
		pending = 0
	}

	area.RecoveredCount = recovered
	area.PendingCount = pending

	switch {
	case recovered >= total && total > 0:
		if area.Status != StatusRestored {
			now := time.Now()
			area.AllRestoredAt = &now
		}
		area.Status = StatusRestored
	default:
		// 出现返修/改判导致不再是全部恢复时, 回退到处置中并清空全部恢复时刻。
		if area.Status == StatusRestored {
			area.AllRestoredAt = nil
		}
		area.Status = StatusDispatched
	}
	return s.repo.Update(ctx, area)
}

// buildDetail 组装主单与明细。
func (s *Service) buildDetail(ctx context.Context, area *AreaFault) (*AreaFaultDetail, error) {
	items, err := s.repo.ListItems(ctx, area.ID)
	if err != nil {
		return nil, err
	}
	area.FillRestoreDuration()
	return &AreaFaultDetail{AreaFault: area, Items: items}, nil
}

// FillRestoreDuration 计算从上报到全部恢复的耗时(分钟), 未落库。
func (a *AreaFault) FillRestoreDuration() {
	if a.AllRestoredAt == nil {
		a.RestoreMinutes = nil
		return
	}
	minutes := int64(a.AllRestoredAt.Sub(a.ReportedAt).Minutes())
	if minutes < 0 {
		minutes = 0
	}
	a.RestoreMinutes = &minutes
}

// dedupPositive 去重并保序返回正整数 ID。
func dedupPositive(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// findMissingLampIDs 找出请求中存在但台账不存在的路灯 ID。
func findMissingLampIDs(requested []uint, devices []lamp.Lamp) []uint {
	existing := make(map[uint]struct{}, len(devices))
	for _, device := range devices {
		existing[device.ID] = struct{}{}
	}
	missing := make([]uint, 0)
	for _, id := range requested {
		if _, ok := existing[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing
}

// isValidFaultType 校验故障类型是否在故障模块允许范围内。
func isValidFaultType(value string) bool {
	for _, item := range fault.FaultTypes() {
		if item == value {
			return true
		}
	}
	return false
}

// areaNoPrefix 生成区域单号前缀, 例如 QY20260920。
func areaNoPrefix(reportedAt time.Time) string {
	return "QY" + reportedAt.Format("20060102")
}

// parseReportedAt 解析上报时间, 为空时取当前时间。
func parseReportedAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now(), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, apperr.BadRequest("上报时间格式不正确, 建议使用 YYYY-MM-DD HH:mm:ss: %s", value)
}
