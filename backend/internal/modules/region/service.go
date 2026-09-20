package region

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/lamp"
	"streetlight/internal/modules/repair"
	"streetlight/pkg/dbtx"
	"streetlight/pkg/pagination"
)

// regionSortSpec 定义区域故障列表允许的排序字段白名单。
var regionSortSpec = pagination.SortSpec{
	Allowed: map[string]string{
		"no":             "no",
		"reported_at":    "reported_at",
		"status":         "status",
		"cause":          "cause",
		"circuit_code":   "circuit_code",
		"affected_count": "affected_count",
		"restored_count": "restored_count",
		"created_at":     "created_at",
	},
	Default: "reported_at",
}

// Service 承载区域故障的业务规则: 建立、关联同回路路灯、统一派工、逐盏登记、整体闭环。
type Service struct {
	repo       *Repository
	lamps      *lamp.Repository
	faults     *fault.Service
	repairs    *repair.Service
	repairRepo *repair.Repository
}

// NewService 构造区域故障服务。
func NewService(repo *Repository, lamps *lamp.Repository, faults *fault.Service, repairs *repair.Service, repairRepo *repair.Repository) *Service {
	return &Service{repo: repo, lamps: lamps, faults: faults, repairs: repairs, repairRepo: repairRepo}
}

// List 分页查询区域故障单。
func (s *Service) List(ctx context.Context, query ListQuery) ([]RegionFault, int64, pagination.Query, error) {
	page := pagination.Parse(query.Params, regionSortSpec)
	filter, err := buildFilter(query)
	if err != nil {
		return nil, 0, page, err
	}
	items, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, 0, page, err
	}
	return items, total, page, nil
}

// Get 查询区域故障主单。
func (s *Service) Get(ctx context.Context, id uint) (*RegionFault, error) {
	return s.repo.GetByID(ctx, id)
}

// Detail 组装区域故障详情, 包含每盏受影响路灯的逐盏处置进展。
func (s *Service) Detail(ctx context.Context, id uint) (*Detail, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.buildItems(ctx, entity)
	if err != nil {
		return nil, err
	}
	return &Detail{RegionFault: entity, Items: items}, nil
}

// Create 建立区域故障并关联同一回路下受影响的多盏路灯, 同时为每盏路灯登记一条子故障。
func (s *Service) Create(ctx context.Context, req CreateRequest) (*RegionFault, error) {
	cause := strings.TrimSpace(req.Cause)
	if !IsValidCause(cause) {
		return nil, apperr.BadRequest("非法的区域故障成因: %s, 仅支持线路故障 / 控制箱故障", cause)
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		return nil, apperr.BadRequest("故障描述不能为空")
	}

	devices, err := s.resolveAffectedLamps(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.ensureLampsAvailable(ctx, devices); err != nil {
		return nil, err
	}

	level := strings.TrimSpace(req.FaultLevel)
	if level == "" {
		level = fault.LevelHigh // 区域性故障默认按紧急处理
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = fault.SourceInspection
	}
	reportedAt, err := parseTime(req.ReportedAt, time.Now())
	if err != nil {
		return nil, err
	}

	entity := &RegionFault{
		No:            "",
		CircuitCode:   devices[0].CircuitCode,
		RoadName:      devices[0].RoadName,
		Cause:         cause,
		FaultLevel:    level,
		Source:        source,
		Description:   description,
		Reporter:      strings.TrimSpace(req.Reporter),
		ReporterPhone: strings.TrimSpace(req.ReporterPhone),
		ReportedAt:    reportedAt,
		Status:        StatusPending,
		AffectedCount: len(devices),
		RestoredCount: 0,
	}

	err = dbtx.InTransaction(ctx, s.repo.DB(), func(txCtx context.Context) error {
		if err := s.repo.CreateWithUniqueNo(txCtx, entity, "QY"+reportedAt.Format("20060102")); err != nil {
			return err
		}
		for _, device := range devices {
			if _, err := s.faults.CreateRegional(txCtx, fault.CreateRequest{
				LampID:        device.ID,
				FaultType:     cause,
				FaultLevel:    level,
				Source:        source,
				Description:   description,
				Reporter:      entity.Reporter,
				ReporterPhone: entity.ReporterPhone,
				ReportedAt:    reportedAt.Format("2006-01-02 15:04:05"),
			}, entity.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

// AddLamps 在派工前向区域故障单追加受影响路灯。
func (s *Service) AddLamps(ctx context.Context, id uint, lampIDs []uint) error {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if entity.Status != StatusPending {
		return apperr.Conflict("区域故障单 %s 已派工, 不允许再追加路灯", entity.No)
	}

	devices, err := s.lamps.ListByIDs(ctx, uniqueIDs(lampIDs))
	if err != nil {
		return err
	}
	if len(devices) != len(uniqueIDs(lampIDs)) {
		return apperr.BadRequest("部分路灯不存在, 请刷新后重试")
	}
	for _, device := range devices {
		if device.CircuitCode != entity.CircuitCode {
			return apperr.BadRequest("路灯 %s 不属于回路 %s, 区域故障只能关联同一回路的路灯", device.Code, entity.CircuitCode)
		}
		if device.RunStatus == lamp.RunStatusOffline {
			return apperr.BadRequest("路灯 %s 已停用, 不纳入区域故障", device.Code)
		}
	}
	if err := s.ensureLampsAvailable(ctx, devices); err != nil {
		return err
	}

	existings, err := s.faults.Repository().ListByRegion(ctx, id)
	if err != nil {
		return err
	}
	existed := make(map[uint]bool, len(existings))
	for _, item := range existings {
		existed[item.LampID] = true
	}

	return dbtx.InTransaction(ctx, s.repo.DB(), func(txCtx context.Context) error {
		added := 0
		for _, device := range devices {
			if existed[device.ID] {
				continue
			}
			if _, err := s.faults.CreateRegional(txCtx, fault.CreateRequest{
				LampID:      device.ID,
				FaultType:   entity.Cause,
				FaultLevel:  entity.FaultLevel,
				Source:      entity.Source,
				Description: entity.Description,
				Reporter:    entity.Reporter,
				ReportedAt:  entity.ReportedAt.Format("2006-01-02 15:04:05"),
			}, entity.ID); err != nil {
				return err
			}
			added++
		}
		if added == 0 {
			return apperr.BadRequest("所选路灯均已在该区域故障单内")
		}
		entity.AffectedCount += added
		return s.repo.Update(txCtx, entity)
	})
}

// RemoveLamp 派工前移除误关联的路灯(同时删除其尚未开工的子故障)。
func (s *Service) RemoveLamp(ctx context.Context, id, faultID uint) error {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if entity.Status != StatusPending {
		return apperr.Conflict("区域故障单 %s 已派工, 不允许移除关联路灯", entity.No)
	}
	child, err := s.faults.GetByID(ctx, faultID)
	if err != nil {
		return err
	}
	if child.RegionID == nil || *child.RegionID != id {
		return apperr.BadRequest("故障 %s 不属于区域故障单 %s", child.FaultNo, entity.No)
	}
	if child.Status != fault.StatusPending {
		return apperr.Conflict("路灯 %s 已进入处置流程, 不允许移除", child.LampCode)
	}

	return dbtx.InTransaction(ctx, s.repo.DB(), func(txCtx context.Context) error {
		if err := s.faults.Repository().Delete(txCtx, faultID); err != nil {
			return err
		}
		entity.AffectedCount--
		if entity.AffectedCount < 0 {
			entity.AffectedCount = 0
		}
		return s.repo.Update(txCtx, entity)
	})
}

// Dispatch 统一派工: 一次派工对区域单内全部受影响路灯开工, 生成逐盏维修记录。
func (s *Service) Dispatch(ctx context.Context, id uint, req DispatchRequest) (*RegionFault, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity.Status == StatusClosed {
		return nil, apperr.Conflict("区域故障单 %s 已闭环, 不允许派工", entity.No)
	}
	if entity.Status == StatusProcessing {
		return nil, apperr.Conflict("区域故障单 %s 已派工, 未恢复的路灯可逐盏再次派工", entity.No)
	}

	repairman := strings.TrimSpace(req.Repairman)
	if repairman == "" {
		return nil, apperr.BadRequest("维修人员不能为空")
	}
	startedAt, err := parseTime(req.StartedAt, time.Now())
	if err != nil {
		return nil, err
	}

	err = dbtx.InTransaction(ctx, s.repo.DB(), func(txCtx context.Context) error {
		children, err := s.faults.Repository().ListByRegion(txCtx, id)
		if err != nil {
			return err
		}
		if len(children) == 0 {
			return apperr.BadRequest("区域故障单 %s 下没有关联路灯, 无法派工", entity.No)
		}
		for _, child := range children {
			if child.Status != fault.StatusPending {
				continue
			}
			if _, err := s.repairs.CreateForRegion(txCtx, repair.CreateRequest{
				FaultID:      child.ID,
				Repairman:    repairman,
				RepairTeam:   strings.TrimSpace(req.RepairTeam),
				ContactPhone: strings.TrimSpace(req.ContactPhone),
				StartedAt:    startedAt.Format("2006-01-02 15:04:05"),
				Content:      strings.TrimSpace(req.Content),
			}); err != nil {
				return err
			}
		}

		entity.Status = StatusProcessing
		entity.DispatchedAt = &startedAt
		entity.DispatchRepairman = repairman
		entity.DispatchTeam = strings.TrimSpace(req.RepairTeam)
		entity.ContactPhone = strings.TrimSpace(req.ContactPhone)
		return s.repo.Update(txCtx, entity)
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

// ResolveLamp 逐盏登记处置结果: 完工一盏登记一盏, 已修复的路灯计入恢复数量。
func (s *Service) ResolveLamp(ctx context.Context, id, faultID uint, req ResolveLampRequest) (*RegionLampItem, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity.Status == StatusClosed {
		return nil, apperr.Conflict("区域故障单 %s 已闭环, 不允许再登记处置结果", entity.No)
	}
	child, err := s.requireChild(ctx, id, faultID)
	if err != nil {
		return nil, err
	}
	if child.Status == fault.StatusRepaired {
		return nil, apperr.Conflict("路灯 %s 已登记为已修复, 等待区域单整体闭环", child.LampCode)
	}

	ongoing, err := s.repairRepo.GetOngoingByFault(ctx, faultID)
	if err != nil {
		return nil, err
	}
	if ongoing == nil {
		return nil, apperr.Conflict("路灯 %s 当前没有进行中的维修, 请先对该盏再次派工", child.LampCode)
	}

	finishedReq := repair.FinishRequest{
		FinishedAt: req.FinishedAt,
		Result:     req.Result,
		Content:    req.Content,
		Materials:  req.Materials,
		Cost:       req.Cost,
		Remark:     req.Remark,
	}
	if err := dbtx.InTransaction(ctx, s.repo.DB(), func(txCtx context.Context) error {
		if _, err := s.repairs.Finish(txCtx, ongoing.ID, finishedReq); err != nil {
			return err
		}
		return s.recalcProgress(txCtx, entity)
	}); err != nil {
		return nil, err
	}
	return s.itemOf(ctx, entity, faultID)
}

// RedispatchLamp 对处置后仍未恢复的单盏路灯再次派工(返修)。
func (s *Service) RedispatchLamp(ctx context.Context, id, faultID uint, req RedispatchRequest) (*RegionLampItem, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity.Status != StatusProcessing {
		return nil, apperr.Conflict("区域故障单 %s 当前状态为 %s, 请先统一派工", entity.No, StatusLabel(entity.Status))
	}
	child, err := s.requireChild(ctx, id, faultID)
	if err != nil {
		return nil, err
	}
	if child.Status == fault.StatusRepaired {
		return nil, apperr.Conflict("路灯 %s 已恢复, 无需再次派工", child.LampCode)
	}
	ongoing, err := s.repairRepo.GetOngoingByFault(ctx, faultID)
	if err != nil {
		return nil, err
	}
	if ongoing != nil {
		return nil, apperr.Conflict("路灯 %s 已有进行中的维修 %s, 请先登记处置结果", child.LampCode, ongoing.RepairNo)
	}

	startedAt, err := parseTime(req.StartedAt, time.Now())
	if err != nil {
		return nil, err
	}
	if _, err := s.repairs.CreateForRegion(ctx, repair.CreateRequest{
		FaultID:      faultID,
		Repairman:    strings.TrimSpace(req.Repairman),
		RepairTeam:   strings.TrimSpace(req.RepairTeam),
		ContactPhone: strings.TrimSpace(req.ContactPhone),
		StartedAt:    startedAt.Format("2006-01-02 15:04:05"),
		Content:      strings.TrimSpace(req.Content),
	}); err != nil {
		return nil, err
	}
	return s.itemOf(ctx, entity, faultID)
}

// Close 区域故障整体闭环: 仅当全部受影响路灯都已恢复时允许闭环。
func (s *Service) Close(ctx context.Context, id uint, req CloseRequest) (*RegionFault, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity.Status == StatusClosed {
		return nil, apperr.Conflict("区域故障单 %s 已闭环, 无需重复操作", entity.No)
	}

	err = dbtx.InTransaction(ctx, s.repo.DB(), func(txCtx context.Context) error {
		children, err := s.faults.Repository().ListByRegion(txCtx, id)
		if err != nil {
			return err
		}
		if len(children) == 0 {
			return apperr.Conflict("区域故障单 %s 下没有关联路灯, 无法闭环", entity.No)
		}

		legacy := 0
		for _, child := range children {
			if child.Status == fault.StatusPending || child.Status == fault.StatusProcessing {
				legacy++
			}
		}
		if legacy > 0 {
			return apperr.Conflict("仍有 %d 盏路灯未恢复, 未全部恢复前不允许整体闭环", legacy)
		}

		for _, child := range children {
			if child.Status == fault.StatusClosed {
				continue
			}
			if _, err := s.faults.CloseForRegion(txCtx, child.ID, fault.CloseRequest{
				Remark: "区域故障单 " + entity.No + " 整体闭环",
			}); err != nil {
				return err
			}
		}

		if err := s.recalcProgress(txCtx, entity); err != nil {
			return err
		}
		now := time.Now()
		entity.Status = StatusClosed
		entity.ClosedAt = &now
		entity.CloseRemark = strings.TrimSpace(req.Remark)
		return s.repo.Update(txCtx, entity)
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

// CircuitOverview 按回路汇总区域故障的影响范围、恢复耗时与遗留数量。
func (s *Service) CircuitOverview(ctx context.Context) (*CircuitOverview, error) {
	regions, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	type agg struct {
		stat       CircuitStat
		roads      map[string]bool
		durations  []float64
		lastReport time.Time
	}
	grouped := map[string]*agg{}
	order := make([]string, 0)

	for index := range regions {
		item := regions[index]
		bucket, ok := grouped[item.CircuitCode]
		if !ok {
			bucket = &agg{stat: CircuitStat{CircuitCode: item.CircuitCode}, roads: map[string]bool{}}
			grouped[item.CircuitCode] = bucket
			order = append(order, item.CircuitCode)
		}

		bucket.stat.RegionTotal++
		if item.Status != StatusClosed {
			bucket.stat.OpenTotal++
			bucket.stat.HasActiveRegion = true
		}
		bucket.stat.AffectedTotal += int64(item.AffectedCount)
		bucket.stat.RestoredTotal += int64(item.RestoredCount)
		if item.RoadName != "" {
			bucket.roads[item.RoadName] = true
		}
		if item.ReportedAt.After(bucket.lastReport) {
			bucket.lastReport = item.ReportedAt
		}
		if item.RestoredAt != nil {
			hours := item.RestoredAt.Sub(item.ReportedAt).Hours()
			if hours >= 0 {
				bucket.durations = append(bucket.durations, hours)
			}
		}
	}

	overview := &CircuitOverview{TotalRegions: int64(len(regions)), Circuits: make([]CircuitStat, 0, len(order))}
	for _, code := range order {
		bucket := grouped[code]
		bucket.stat.LegacyTotal = bucket.stat.AffectedTotal - bucket.stat.RestoredTotal
		if len(bucket.durations) > 0 {
			sum := 0.0
			for _, value := range bucket.durations {
				sum += value
			}
			bucket.stat.AvgRecoveryHours = round2(sum / float64(len(bucket.durations)))
		}
		bucket.stat.LastReportedAt = bucket.lastReport
		for road := range bucket.roads {
			bucket.stat.RoadNames = append(bucket.stat.RoadNames, road)
		}
		sort.Strings(bucket.stat.RoadNames)

		overview.OpenRegions += bucket.stat.OpenTotal
		overview.AffectedLamps += bucket.stat.AffectedTotal
		overview.RestoredLamps += bucket.stat.RestoredTotal
		overview.LegacyLamps += bucket.stat.LegacyTotal
		overview.Circuits = append(overview.Circuits, bucket.stat)
	}

	sort.SliceStable(overview.Circuits, func(i, j int) bool {
		a, b := overview.Circuits[i], overview.Circuits[j]
		if a.HasActiveRegion != b.HasActiveRegion {
			return a.HasActiveRegion
		}
		if a.LegacyTotal != b.LegacyTotal {
			return a.LegacyTotal > b.LegacyTotal
		}
		return grouped[a.CircuitCode].lastReport.After(grouped[b.CircuitCode].lastReport)
	})
	return overview, nil
}

// Metadata 返回区域故障模块字典。
func (s *Service) Metadata() *Meta {
	return &Meta{
		Statuses: Statuses(),
		Causes:   Causes(),
		Levels:   fault.Levels(),
		Sources:  fault.Sources(),
	}
}

// requireChild 校验子故障归属该区域单。
func (s *Service) requireChild(ctx context.Context, regionID, faultID uint) (*fault.Fault, error) {
	child, err := s.faults.GetByID(ctx, faultID)
	if err != nil {
		return nil, err
	}
	if child.RegionID == nil || *child.RegionID != regionID {
		return nil, apperr.BadRequest("故障 %s 不属于该区域故障单", child.FaultNo)
	}
	return child, nil
}

// recalcProgress 依据子故障最新状态重算区域单的恢复数量, 全部恢复时记录恢复时间。
func (s *Service) recalcProgress(ctx context.Context, entity *RegionFault) error {
	children, err := s.faults.Repository().ListByRegion(ctx, entity.ID)
	if err != nil {
		return err
	}
	restored := 0
	for _, child := range children {
		if child.Status == fault.StatusRepaired || child.Status == fault.StatusClosed {
			restored++
		}
	}
	entity.RestoredCount = restored
	if restored == len(children) && entity.RestoredAt == nil {
		now := time.Now()
		entity.RestoredAt = &now
	}
	return s.repo.Update(ctx, entity)
}

// resolveAffectedLamps 解析受影响路灯: 显式指定时校验同一回路, 否则取整条回路。
func (s *Service) resolveAffectedLamps(ctx context.Context, req CreateRequest) ([]lamp.Lamp, error) {
	if len(req.LampIDs) > 0 {
		devices, err := s.lamps.ListByIDs(ctx, uniqueIDs(req.LampIDs))
		if err != nil {
			return nil, err
		}
		if len(devices) != len(uniqueIDs(req.LampIDs)) {
			return nil, apperr.BadRequest("部分路灯不存在, 请刷新后重试")
		}
		circuit := devices[0].CircuitCode
		if circuit == "" {
			return nil, apperr.BadRequest("路灯 %s 未登记所属回路, 无法建立区域故障", devices[0].Code)
		}
		for _, device := range devices {
			if device.CircuitCode != circuit {
				return nil, apperr.BadRequest("区域故障只能关联同一回路的路灯, %s 属于回路 %s, 与 %s 不一致", device.Code, device.CircuitCode, circuit)
			}
			if device.RunStatus == lamp.RunStatusOffline {
				return nil, apperr.BadRequest("路灯 %s 已停用, 不纳入区域故障", device.Code)
			}
		}
		return devices, nil
	}

	circuit := strings.TrimSpace(req.CircuitCode)
	if circuit == "" {
		return nil, apperr.BadRequest("请选择受影响回路, 或显式指定受影响路灯")
	}
	devices, err := s.lamps.ListByCircuit(ctx, circuit)
	if err != nil {
		return nil, err
	}
	active := make([]lamp.Lamp, 0, len(devices))
	for _, device := range devices {
		if device.RunStatus != lamp.RunStatusOffline {
			active = append(active, device)
		}
	}
	if len(active) == 0 {
		return nil, apperr.BadRequest("回路 %s 下没有在运路灯, 无法建立区域故障", circuit)
	}
	return active, nil
}

// ensureLampsAvailable 校验待关联路灯当前都没有未闭环故障。
func (s *Service) ensureLampsAvailable(ctx context.Context, devices []lamp.Lamp) error {
	for _, device := range devices {
		count, err := s.faults.Repository().CountOpenByLamp(ctx, device.ID)
		if err != nil {
			return err
		}
		if count > 0 {
			return apperr.Conflict("路灯 %s 已存在未闭环故障, 请先处理后再纳入区域故障", device.Code)
		}
	}
	return nil
}

// buildItems 组装区域单下全部路灯的处置进展。
func (s *Service) buildItems(ctx context.Context, entity *RegionFault) ([]RegionLampItem, error) {
	children, err := s.faults.Repository().ListByRegion(ctx, entity.ID)
	if err != nil {
		return nil, err
	}
	lampIDs := make([]uint, 0, len(children))
	for _, child := range children {
		lampIDs = append(lampIDs, child.LampID)
	}
	lamps, err := s.lamps.ListByIDs(ctx, lampIDs)
	if err != nil {
		return nil, err
	}
	lampMap := make(map[uint]lamp.Lamp, len(lamps))
	for _, device := range lamps {
		lampMap[device.ID] = device
	}

	items := make([]RegionLampItem, 0, len(children))
	for _, child := range children {
		item := RegionLampItem{
			FaultID:     child.ID,
			FaultNo:     child.FaultNo,
			FaultStatus: child.Status,
			LampID:      child.LampID,
			LampCode:    child.LampCode,
			RoadName:    child.RoadName,
			Restored:    child.Status == fault.StatusRepaired || child.Status == fault.StatusClosed,
		}
		if device, ok := lampMap[child.LampID]; ok {
			item.LampName = device.Name
			item.RunStatus = device.RunStatus
		}
		latest, err := s.repairRepo.LatestByFault(ctx, child.ID)
		if err != nil {
			return nil, err
		}
		if latest != nil {
			item.RepairID = &latest.ID
			item.RepairNo = latest.RepairNo
			item.Repairman = latest.Repairman
			item.RepairStatus = latest.Status
			item.RepairResult = latest.Result
			started := latest.StartedAt
			item.StartedAt = &started
			item.FinishedAt = latest.FinishedAt
		}
		items = append(items, item)
	}
	return items, nil
}

// itemOf 组装单盏路灯的最新处置进展。
func (s *Service) itemOf(ctx context.Context, entity *RegionFault, faultID uint) (*RegionLampItem, error) {
	items, err := s.buildItems(ctx, entity)
	if err != nil {
		return nil, err
	}
	for index := range items {
		if items[index].FaultID == faultID {
			return &items[index], nil
		}
	}
	return nil, apperr.NotFound("区域故障单 %s 下未找到故障 id=%d", entity.No, faultID)
}

// buildFilter 将列表查询参数转换为仓储条件并解析日期区间。
func buildFilter(query ListQuery) (Filter, error) {
	filter := Filter{
		Keyword:    strings.TrimSpace(query.Keyword),
		Status:     strings.TrimSpace(query.Status),
		Cause:      strings.TrimSpace(query.Cause),
		RoadName:   strings.TrimSpace(query.RoadName),
		RepairTeam: strings.TrimSpace(query.RepairTeam),
		OnlyOpen:   query.OnlyOpen,
	}
	if filter.Status != "" && !IsValidStatus(filter.Status) {
		return filter, apperr.BadRequest("非法的区域故障状态: %s", filter.Status)
	}
	if filter.Cause != "" && !IsValidCause(filter.Cause) {
		return filter, apperr.BadRequest("非法的区域故障成因: %s", filter.Cause)
	}

	if value := strings.TrimSpace(query.StartDate); value != "" {
		from, err := parseDay(value)
		if err != nil {
			return filter, err
		}
		filter.ReportedFrom = &from
	}
	if value := strings.TrimSpace(query.EndDate); value != "" {
		to, err := parseDay(value)
		if err != nil {
			return filter, err
		}
		to = to.AddDate(0, 0, 1)
		filter.ReportedTo = &to
	}
	if filter.ReportedFrom != nil && filter.ReportedTo != nil && filter.ReportedTo.Before(*filter.ReportedFrom) {
		return filter, apperr.BadRequest("结束日期不能早于开始日期")
	}
	return filter, nil
}

// parseTime 解析时间字符串, 为空时返回 fallback。
func parseTime(value string, fallback time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, apperr.BadRequest("时间格式不正确, 建议使用 YYYY-MM-DD HH:mm:ss: %s", value)
}

// parseDay 解析 YYYY-MM-DD 日期, 返回当天零点。
func parseDay(value string) (time.Time, error) {
	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, apperr.BadRequest("日期格式应为 YYYY-MM-DD, 当前值: %s", value)
	}
	return date, nil
}

// uniqueIDs 对路灯 ID 去名并保持顺序。
func uniqueIDs(ids []uint) []uint {
	seen := make(map[uint]bool, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

// round2 保留两位小数。
func round2(value float64) float64 {
	if value < 0 {
		value = 0
	}
	return math.Round(value*100) / 100
}
