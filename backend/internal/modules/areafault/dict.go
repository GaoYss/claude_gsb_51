package areafault

var statusLabels = map[string]string{
	StatusPending:    "待派工",
	StatusDispatched: "处置中",
	StatusRestored:   "全部恢复",
	StatusClosed:     "已闭环",
}

var causeLabels = map[string]string{
	CauseLine:    "线路故障",
	CauseCabinet: "控制箱故障",
}

var itemResultLabels = map[string]string{
	ItemResultPending:   "待处置",
	ItemResultRecovered: "已恢复",
	ItemResultParts:     "待配件",
	ItemResultObserving: "观察中",
	ItemResultUnfixable: "无法修复",
}

// StatusLabel 返回区域故障状态的中文名称。
func StatusLabel(status string) string {
	if label, ok := statusLabels[status]; ok {
		return label
	}
	return status
}

// CauseLabel 返回诱因中文名称。
func CauseLabel(cause string) string {
	if label, ok := causeLabels[cause]; ok {
		return label
	}
	return cause
}

// ItemResultLabel 返回逐盏处置结果的中文名称。
func ItemResultLabel(result string) string {
	if label, ok := itemResultLabels[result]; ok {
		return label
	}
	return result
}
