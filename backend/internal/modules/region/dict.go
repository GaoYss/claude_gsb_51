package region

var statusLabels = map[string]string{
	StatusPending:    "待派工",
	StatusProcessing: "处置中",
	StatusClosed:     "已闭环",
}

// StatusLabel 返回区域故障状态的中文名称。
func StatusLabel(status string) string {
	if label, ok := statusLabels[status]; ok {
		return label
	}
	return status
}
