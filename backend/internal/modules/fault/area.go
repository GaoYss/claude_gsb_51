package fault

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"streetlight/internal/apperr"
)

// CreateForArea 供区域故障模块批量建单: 为同一回路的多盏路灯各登记一条故障。
// 入参已在区域故障服务中完成归一化与批量校验, 这里仅负责落库与路灯状态联动。
func (s *Service) CreateForArea(
	ctx context.Context,
	lampID uint,
	faultType string,
	level string,
	description string,
	reporter string,
	reportedAt time.Time,
) (*Fault, error) {
	device, err := s.lamps.Get(ctx, lampID)
	if err != nil {
		return nil, err
	}

	openCount, err := s.repo.CountOpenByLamp(ctx, device.ID)
	if err != nil {
		return nil, err
	}
	if openCount > 0 {
		return nil, apperr.Conflict("路灯 %s 已存在 %d 条未闭环故障, 请先处理后再登记", device.Code, openCount)
	}

	entity := &Fault{
		LampID:      device.ID,
		LampCode:    device.Code,
		RoadName:    device.RoadName,
		FaultType:   faultType,
		FaultLevel:  level,
		Source:      SourceMonitoring,
		Description: strings.TrimSpace(description),
		Reporter:    strings.TrimSpace(reporter),
		ReportedAt:  reportedAt,
		Status:      StatusPending,
	}

	if err := s.repo.CreateWithUniqueNo(ctx, entity, faultNoPrefix(reportedAt)); err != nil {
		return nil, err
	}
	if err := s.syncLampStatus(ctx, device.ID); err != nil {
		slog.Warn("区域故障建单后同步路灯运行状态失败", "lamp_id", device.ID, "fault_no", entity.FaultNo, "error", err)
	}
	return entity, nil
}

// CloseIfOpen 关闭指定故障: 已关闭直接返回 false; 待处理/已修复可直接闭环返回 true;
// 维修中不允许直接闭环(返回 false), 交由区域故障上层判定为遗留。
func (s *Service) CloseIfOpen(ctx context.Context, id uint, remark string) (bool, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	if entity.Status == StatusClosed {
		return false, nil
	}
	if !canTransitTo(entity.Status, StatusClosed) {
		return false, nil
	}

	now := time.Now()
	entity.Status = StatusClosed
	entity.ClosedAt = &now
	entity.CloseRemark = strings.TrimSpace(remark)
	if err := s.repo.Update(ctx, entity); err != nil {
		return false, err
	}
	if err := s.syncLampStatus(ctx, entity.LampID); err != nil {
		slog.Warn("区域故障闭环后同步路灯运行状态失败", "lamp_id", entity.LampID, "fault_no", entity.FaultNo, "error", err)
	}
	return true, nil
}
