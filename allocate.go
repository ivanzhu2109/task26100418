package autoreach

import "sort"

// Allocate 计算满足全部约束且总误差最小的分配方案。
// 不存在满足约束的方案时返回 ok == false。
//
// 建模：对每条 (活动, ZIP) 目标关系，理想值 v（万分之一美分）分解为
// v = floor*Scale + r（0 <= r < Scale）。r == 0 的关系金额固定为 floor；
// 其余关系金额为 floor+y，y ∈ {0,1}。设活动 c 需要的进位次数
// k_c = Budget - Σfloor，ZIP z 的剩余容量 u_z = cap_z - Σfloor，则问题等价于：
//
//	每个活动恰好选 k_c 条关系进位，每个 ZIP 被选中的关系数不超过 u_z，
//	最小化 Σ y*(Scale-2r)（总误差 = Σr + 该值）。
//
// 这是二分图上的最小费用流：源→活动(容量 k_c)，活动→ZIP(容量 1, 费用 Scale-2r)，
// ZIP→汇(容量 u_z)。流量必须达到 Σk_c，否则不可行。
func Allocate(campaigns []Campaign, zipCaps map[string]int64) (plan Plan, ok bool) {
	// 固定顺序，保证结果可复现。
	cIdx := make(map[string]int, len(campaigns))
	campaignIDs := make([]string, 0, len(campaigns))
	for _, c := range campaigns {
		if _, dup := cIdx[c.ID]; dup {
			return Plan{}, false
		}
		cIdx[c.ID] = len(campaignIDs)
		campaignIDs = append(campaignIDs, c.ID)
	}
	sort.Strings(campaignIDs)
	for i, id := range campaignIDs {
		cIdx[id] = i
	}
	zipSet := make(map[string]struct{}, len(zipCaps))
	for z := range zipCaps {
		zipSet[z] = struct{}{}
	}
	for _, c := range campaigns {
		for z := range c.Ideals {
			zipSet[z] = struct{}{}
		}
	}
	zips := make([]string, 0, len(zipSet))
	for z := range zipSet {
		zips = append(zips, z)
	}
	sort.Strings(zips)
	zIdx := make(map[string]int, len(zips))
	for i, z := range zips {
		zIdx[z] = i
	}

	nc, nz := len(campaignIDs), len(zips)
	// 节点编号：0=源, 1..nc=活动, nc+1..nc+nz=ZIP, nc+nz+1=汇。
	src, sink := 0, nc+nz+1
	g := make([][]mcfEdge, nc+nz+2)

	floorSum := make([]int64, nz) // 每个 ZIP 所有关系的 floor 之和
	var baseErr int64             // 所有关系向下取整时的误差之和 Σr
	var needFlow int64            // Σk_c

	// 记录每个活动的关系，便于先算 floor 再建边。
	type rel struct {
		z    int
		fl   int64
		cost int64 // Scale-2r
	}
	rels := make([][]rel, nc)
	for _, c := range campaigns {
		ci := cIdx[c.ID]
		var flSum int64
		for z, v := range c.Ideals {
			if v < 0 {
				return Plan{}, false
			}
			fl, r := v/Scale, v%Scale
			flSum += fl
			floorSum[zIdx[z]] += fl
			baseErr += r
			if r > 0 {
				rels[ci] = append(rels[ci], rel{z: zIdx[z], fl: fl, cost: Scale - 2*r})
			}
		}
		k := c.Budget - flSum
		if k < 0 || k > int64(len(rels[ci])) {
			return Plan{}, false // 输入不满足“理想值之和=预算*Scale”或无法凑整
		}
		addEdge(g, src, 1+ci, k, 0)
		needFlow += k
	}
	for _, c := range campaigns {
		ci := cIdx[c.ID]
		for _, rl := range rels[ci] {
			addEdge(g, 1+ci, 1+nc+rl.z, 1, rl.cost)
		}
	}
	for zi := 0; zi < nz; zi++ {
		u := zipCaps[zips[zi]] - floorSum[zi]
		if u < 0 {
			return Plan{}, false // 仅向下取整就已超出 ZIP 上限
		}
		addEdge(g, 1+nc+zi, sink, u, 0)
	}

	flow, cost := minCostFlow(g, src, sink, needFlow)
	if flow < needFlow {
		return Plan{}, false
	}

	// 从流图上读出每个活动→ZIP 边上的流量（即是否进位）。
	amounts := make(map[string]map[string]int64, nc)
	for _, c := range campaigns {
		amounts[c.ID] = make(map[string]int64, len(c.Ideals))
		for z, v := range c.Ideals {
			amounts[c.ID][z] = v / Scale
		}
	}
	for _, c := range campaigns {
		ci := cIdx[c.ID]
		for _, e := range g[1+ci] {
			if e.to <= nc || e.to > nc+nz {
				continue // 源方向的反向边
			}
			// 正向边容量 1，剩余容量 0 表示被选中进位。
			if e.cap == 0 {
				amounts[c.ID][zips[e.to-1-nc]]++
			}
		}
	}
	return Plan{Amounts: amounts, TotalError: baseErr + cost}, true
}
