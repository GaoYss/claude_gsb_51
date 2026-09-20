package areafault

import (
	"context"
	"math"
)

// LabelCount 通用分组统计项。
type LabelCount struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// CircuitOverviewRow 按回路聚合的一行概览。
type CircuitOverviewRow struct {
	CircuitName        string `json:"circuit_name"`
	RoadName           string `json:"road_name"`
	OpenTotal          int64  `json:"open_total"`
	ClosedTotal        int64  `json:"closed_total"`
	AffectedLampCount  int64  `json:"affected_lamp_count"`  // 影响范围: 累计受影响灯数
	RecoveredLampCount int64  `json:"recovered_lamp_count"` // 已恢复灯数
	PendingLampCount   int64  `json:"pending_lamp_count"`   // 遗留数量: 尚未恢复灯数
	AvgRestoreMinutes  *int64 `json:"avg_restore_minutes"`  // 平均恢复耗时(分钟), 无恢复样本时为空
}

// Overview 区域故障概览。
type Overview struct {
	Total          int64                `json:"total"`
	OpenTotal      int64                `json:"open_total"`
	ClosedTotal    int64                `json:"closed_total"`
	AffectedLamps  int64                `json:"affected_lamps"`
	RecoveredLamps int64                `json:"recovered_lamps"`
	PendingLamps   int64                `json:"pending_lamps"`
	ByCause        []LabelCount         `json:"by_cause"`
	ByStatus       []LabelCount         `json:"by_status"`
	Circuits       []CircuitOverviewRow `json:"circuits"`
}

// CircuitOverview 按回路给出影响范围、恢复耗时与遗留数量。
func (s *Service) CircuitOverview(ctx context.Context) (*Overview, error) {
	rows, err := s.repo.CircuitStats(ctx)
	if err != nil {
		return nil, err
	}
	durations, err := s.repo.RestoreDurationByCircuit(ctx)
	if err != nil {
		return nil, err
	}

	circuits := make([]CircuitOverviewRow, 0, len(rows))
	for _, row := range rows {
		circuits = append(circuits, CircuitOverviewRow{
			CircuitName:        row.CircuitName,
			RoadName:           row.RoadName,
			OpenTotal:          row.OpenTotal,
			ClosedTotal:        row.ClosedTotal,
			AffectedLampCount:  row.AffectedLamps,
			RecoveredLampCount: row.RecoveredLamps,
			PendingLampCount:   row.PendingLamps,
			AvgRestoreMinutes:  averageMinutes(durations[row.CircuitName]),
		})
	}

	overview := &Overview{
		ByCause:  make([]LabelCount, 0, len(Causes())),
		ByStatus: make([]LabelCount, 0, len(Statuses())),
		Circuits: circuits,
	}
	for _, item := range circuits {
		overview.Total += item.OpenTotal + item.ClosedTotal
		overview.OpenTotal += item.OpenTotal
		overview.ClosedTotal += item.ClosedTotal
		overview.AffectedLamps += item.AffectedLampCount
		overview.RecoveredLamps += item.RecoveredLampCount
		overview.PendingLamps += item.PendingLampCount
	}

	causeCounts, statusCounts, err := s.repo.GroupByCauseAndStatus(ctx)
	if err != nil {
		return nil, err
	}
	overview.ByCause = orderedCounts(causeCounts, Causes())
	overview.ByStatus = orderedCounts(statusCounts, Statuses())
	return overview, nil
}

// averageMinutes 在应用层计算某回路已恢复区域故障的平均恢复耗时(分钟), 保证跨数据库一致。
func averageMinutes(samples []RestoreDuration) *int64 {
	var sum int64
	count := 0
	for _, sample := range samples {
		if sample.AllRestoredAt == nil {
			continue
		}
		minutes := int64(sample.AllRestoredAt.Sub(sample.ReportedAt).Minutes())
		if minutes < 0 {
			minutes = 0
		}
		sum += minutes
		count++
	}
	if count == 0 {
		return nil
	}
	average := int64(math.Round(float64(sum) / float64(count)))
	return &average
}

// orderedCounts 按给定顺序输出分组统计, 保证前端展示顺序稳定且包含零值项。
func orderedCounts(counts map[string]int64, order []string) []LabelCount {
	result := make([]LabelCount, 0, len(order))
	for _, label := range order {
		result = append(result, LabelCount{Label: label, Count: counts[label]})
	}
	return result
}
