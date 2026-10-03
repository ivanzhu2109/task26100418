package autoreach

// mcfEdge 是最小费用流图中的一条边。
type mcfEdge struct {
	to   int
	rev  int // 在 g[to] 中反向边的下标
	cap  int64
	cost int64
}

// minCostFlow 在图上求从 s 到 t、流量不超过 maxFlow 的最小费用流。
// 返回 (流量, 费用)。每次增广沿最短路推进，使用 SPFA 找最短路，
// 因此允许负费用边（图中无负环即可，本问题建图保证无负环）。
func minCostFlow(g [][]mcfEdge, s, t int, maxFlow int64) (flow int64, cost int64) {
	n := len(g)
	const inf = int64(1) << 62
	for flow < maxFlow {
		dist := make([]int64, n)
		inQ := make([]bool, n)
		prevV := make([]int, n)
		prevE := make([]int, n)
		for i := range dist {
			dist[i] = inf
		}
		dist[s] = 0
		queue := []int{s}
		inQ[s] = true
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			inQ[v] = false
			for ei, e := range g[v] {
				if e.cap > 0 && dist[v]+e.cost < dist[e.to] {
					dist[e.to] = dist[v] + e.cost
					prevV[e.to] = v
					prevE[e.to] = ei
					if !inQ[e.to] {
						inQ[e.to] = true
						queue = append(queue, e.to)
					}
				}
			}
		}
		if dist[t] == inf {
			break
		}
		d := maxFlow - flow
		for v := t; v != s; v = prevV[v] {
			if g[prevV[v]][prevE[v]].cap < d {
				d = g[prevV[v]][prevE[v]].cap
			}
		}
		for v := t; v != s; v = prevV[v] {
			e := &g[prevV[v]][prevE[v]]
			e.cap -= d
			g[v][e.rev].cap += d
		}
		flow += d
		cost += d * dist[t]
	}
	return flow, cost
}

// addEdge 向图中加入一条边及其反向边。
func addEdge(g [][]mcfEdge, from, to int, cap, cost int64) {
	g[from] = append(g[from], mcfEdge{to: to, rev: len(g[to]), cap: cap, cost: cost})
	g[to] = append(g[to], mcfEdge{to: from, rev: len(g[from]) - 1, cap: 0, cost: -cost})
}
