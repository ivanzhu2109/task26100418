package autoreach

import "math"

// mcfEdge 为最小费用流中的一条边。
type mcfEdge struct {
	to   int
	rev  int
	cap  int64
	cost int64
}

// minCostFlow 为邻接表形式的最小费用流图。
type minCostFlow struct {
	graph [][]mcfEdge
}

// newMinCostFlow 创建有 n 个结点的空图。
func newMinCostFlow(n int) *minCostFlow {
	return &minCostFlow{graph: make([][]mcfEdge, n)}
}

// addEdge 加入一条容量为 cap、单位费用为 cost 的有向边及其反向边。
func (g *minCostFlow) addEdge(from, to int, cap, cost int64) {
	g.graph[from] = append(g.graph[from], mcfEdge{to: to, rev: len(g.graph[to]), cap: cap, cost: cost})
	g.graph[to] = append(g.graph[to], mcfEdge{to: from, rev: len(g.graph[from]) - 1, cap: 0, cost: -cost})
}

// flow 在 s->t 上推送至多 limit 单位流量，每次沿最短路增广（SPFA，
// 允许负费用边），返回实际流量与对应的最小总费用。
func (g *minCostFlow) flow(s, t int, limit int64) (flowAmount, cost int64) {
	n := len(g.graph)
	for flowAmount < limit {
		dist := make([]int64, n)
		prevV := make([]int, n)
		prevE := make([]int, n)
		inQueue := make([]bool, n)
		for i := range dist {
			dist[i] = math.MaxInt64
		}
		dist[s] = 0
		queue := []int{s}
		inQueue[s] = true
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			inQueue[v] = false
			for i, e := range g.graph[v] {
				if e.cap > 0 && dist[v] != math.MaxInt64 && dist[e.to] > dist[v]+e.cost {
					dist[e.to] = dist[v] + e.cost
					prevV[e.to] = v
					prevE[e.to] = i
					if !inQueue[e.to] {
						queue = append(queue, e.to)
						inQueue[e.to] = true
					}
				}
			}
		}
		if dist[t] == math.MaxInt64 {
			break
		}
		d := limit - flowAmount
		for v := t; v != s; v = prevV[v] {
			if c := g.graph[prevV[v]][prevE[v]].cap; c < d {
				d = c
			}
		}
		for v := t; v != s; v = prevV[v] {
			e := &g.graph[prevV[v]][prevE[v]]
			e.cap -= d
			g.graph[v][e.rev].cap += d
		}
		flowAmount += d
		cost += d * dist[t]
	}
	return flowAmount, cost
}
