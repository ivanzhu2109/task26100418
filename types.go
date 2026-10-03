// Package autoreach 实现 AutoReach 的月度广告预算预分配组件。
package autoreach

// Scale 表示理想分配值的精度：1 美分 = Scale 个理想值单位。
const Scale int64 = 10000

// Campaign 描述同一车行、同一月份下一个待分配的活动。
type Campaign struct {
	// ID 是活动的唯一标识。
	ID string
	// Budget 是该活动的预算，单位为整数美分。
	Budget int64
	// Ideals 给出该活动在各目标 ZIP 上的理想分配值，
	// 单位为万分之一美分，所有理想值之和恰好等于 Budget*Scale。
	// 未列入的 ZIP 不得接收该活动的预算。
	Ideals map[string]int64
}

// Plan 是一份完整的分配方案。
type Plan struct {
	// Amounts[campaignID][zip] 为该活动分配到该 ZIP 的整数美分金额。
	Amounts map[string]map[string]int64
	// TotalError 是总误差，单位为万分之一美分。
	TotalError int64
}
