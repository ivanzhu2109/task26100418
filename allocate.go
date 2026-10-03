// Package autoreach 实现 AutoReach 的月度广告预算预分配组件。
package autoreach

import "sort"

// quantum 为 1 美分对应的理想值单位数（万分之一美分）。
const quantum = 10000

// Campaign 表示同一车行、同一月份待分配的一个活动。
type Campaign struct {
	ID string
	// Budget 为该活动的预算，单位：整数美分。
	Budget int64
	// Ideals 为该活动在各目标 ZIP 上的理想分配值，
	// 单位：万分之一美分。同一活动的理想值之和恰好等于 Budget*10000。
	Ideals map[string]int64
}

// Allocation 表示一个活动对一个 ZIP 的实际分配金额。
type Allocation struct {
	CampaignID string
	Zip        string
	// Amount 为实际分配金额，单位：整数美分。
	Amount int64
}

// Result 为 Allocate 的分配结果。
type Result struct {
	// Allocations 覆盖所有列入目标集合的 (活动, ZIP) 关系。
	Allocations []Allocation
	// TotalError 为总误差，单位：万分之一美分。
	TotalError int64
}

// Allocate 计算满足全部约束且总误差最小的分配方案。
// 不存在可行方案时返回 ok=false。
//
// 思路：每条 (活动, ZIP) 关系的实际金额只能是理想值换算成美分后的
// floor 或 ceil。先取全部 floor 作为基线，则活动 c 还需再分配
// k_c = (Σ 理想值小数部分)/10000 个整数美分，每个关系至多再承担 1 美分；
// 某关系多承担 1 美分时，误差从 r 变为 10000-r（r 为小数部分），
// 增量费用为 10000-2r（可为负）。ZIP 的剩余容量为 上限-基线。
// 于是问题化为最小费用流：源->活动(容量 k_c)，活动->ZIP(容量 1，
// 费用 10000-2r)，ZIP->汇(容量 剩余容量)。能推送满 Σk_c 当且仅当可行，
// 最小费用对应最小总误差。
func Allocate(campaigns []Campaign, zipCaps map[string]int64) (res Result, ok bool) {
	zipSet := make(map[string]bool)
	for _, c := range campaigns {
		for zip := range c.Ideals {
			zipSet[zip] = true
		}
	}
	zips := make([]string, 0, len(zipSet))
	for zip := range zipSet {
		zips = append(zips, zip)
	}
	sort.Strings(zips)
	zipNode := make(map[string]int, len(zips))

	nC := len(campaigns)
	nZ := len(zips)
	source := 0
	sink := 1 + nC + nZ
	flow := newMinCostFlow(sink + 1)

	baseline := make(map[string]int64, nZ) // 每个 ZIP 已被 floor 占用的容量
	var totalNeed int64
	type rel struct {
		ci, zi   int
		floor, r int64
		edgeIdx  int // 活动结点出边下标，-1 表示无增广边
	}
	var rels []rel
	for ci, c := range campaigns {
		var sumFloor, sumR int64
		for zi, zip := range zips {
			ideal, targeted := c.Ideals[zip]
			if !targeted {
				continue
			}
			if ideal < 0 {
				return Result{}, false
			}
			floor := ideal / quantum
			r := ideal % quantum
			baseline[zip] += floor
			sumFloor += floor
			sumR += r
			edgeIdx := -1
			if r > 0 {
				edgeIdx = len(flow.graph[1+ci])
				flow.addEdge(1+ci, 1+nC+zi, 1, quantum-2*r)
			}
			rels = append(rels, rel{ci: ci, zi: zi, floor: floor, r: r, edgeIdx: edgeIdx})
		}
		if sumR%quantum != 0 {
			return Result{}, false // 输入不满足“理想值之和=预算*10000”
		}
		k := sumR / quantum
		if c.Budget < 0 || sumFloor+k != c.Budget {
			return Result{}, false
		}
		totalNeed += k
		flow.addEdge(source, 1+ci, k, 0)
	}

	for zi, zip := range zips {
		zipNode[zip] = 1 + nC + zi
		remain := zipCaps[zip] - baseline[zip]
		if remain < 0 {
			return Result{}, false // floor 基线已超出 ZIP 上限
		}
		flow.addEdge(1+nC+zi, sink, remain, 0)
	}

	pushed, _ := flow.flow(source, sink, totalNeed)
	if pushed != totalNeed {
		return Result{}, false
	}

	res = Result{Allocations: make([]Allocation, 0, len(rels))}
	for _, rl := range rels {
		amount := rl.floor
		if rl.edgeIdx >= 0 && flow.graph[1+rl.ci][rl.edgeIdx].cap == 0 {
			amount++ // 该关系被增广了 1 美分（floor -> ceil）
		}
		ideal := rl.floor*quantum + rl.r
		diff := amount*quantum - ideal
		if diff < 0 {
			diff = -diff
		}
		res.TotalError += diff
		res.Allocations = append(res.Allocations, Allocation{
			CampaignID: campaigns[rl.ci].ID,
			Zip:        zips[rl.zi],
			Amount:     amount,
		})
	}
	return res, true
}
