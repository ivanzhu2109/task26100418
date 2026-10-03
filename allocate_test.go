package autoreach

import (
	"math/rand"
	"sort"
	"testing"
)

// verifySolution 校验返回方案本身满足全部约束，并返回重算的总误差。
func verifySolution(t *testing.T, campaigns []Campaign, zipCaps map[string]int64, res Result) int64 {
	t.Helper()
	byID := make(map[string]Campaign, len(campaigns))
	for _, c := range campaigns {
		byID[c.ID] = c
	}
	perCampaign := make(map[string]int64, len(campaigns))
	perZip := make(map[string]int64)
	var totalErr int64
	for _, a := range res.Allocations {
		c, ok := byID[a.CampaignID]
		if !ok {
			t.Fatalf("方案包含未知活动 %q", a.CampaignID)
		}
		ideal, targeted := c.Ideals[a.Zip]
		if !targeted {
			t.Fatalf("活动 %q 向未列入目标集合的 ZIP %q 分配了预算", a.CampaignID, a.Zip)
		}
		floor, ceil := ideal/quantum, ideal/quantum
		if ideal%quantum != 0 {
			ceil++
		}
		if a.Amount != floor && a.Amount != ceil {
			t.Fatalf("活动 %q 在 ZIP %q 的金额 %d 不是理想值 %d 的 floor/ceil", a.CampaignID, a.Zip, a.Amount, ideal)
		}
		perCampaign[a.CampaignID] += a.Amount
		perZip[a.Zip] += a.Amount
		diff := a.Amount*quantum - ideal
		if diff < 0 {
			diff = -diff
		}
		totalErr += diff
	}
	for _, c := range campaigns {
		if perCampaign[c.ID] != c.Budget {
			t.Fatalf("活动 %q 实际分配 %d 不等于预算 %d", c.ID, perCampaign[c.ID], c.Budget)
		}
	}
	for zip, got := range perZip {
		if got > zipCaps[zip] {
			t.Fatalf("ZIP %q 收到 %d 超过上限 %d", zip, got, zipCaps[zip])
		}
	}
	if totalErr != res.TotalError {
		t.Fatalf("TotalError=%d 与逐关系重算结果 %d 不一致", res.TotalError, totalErr)
	}
	return totalErr
}

// TestSharedZipRequiresReallocation 多个活动偏向同一 ZIP：
// 逐活动取最接近理想值的取整会超限，必须全局重新分配。
func TestSharedZipRequiresReallocation(t *testing.T) {
	// 两个活动各 60 美分预算，理想值都是 A=59.5、B=0.5（万分之一美分）。
	mk := func(id string) Campaign {
		return Campaign{ID: id, Budget: 60, Ideals: map[string]int64{"A": 595000, "B": 5000}}
	}
	campaigns := []Campaign{mk("c1"), mk("c2")}
	// A 的上限 119：floor 基线 59+59=118 可行，
	// 但逐活动各自四舍五入会给 A 分 60，合计 120 > 119，必须重分配。
	zipCaps := map[string]int64{"A": 119, "B": 1000}

	res, ok := Allocate(campaigns, zipCaps)
	if !ok {
		t.Fatal("应为可行，却返回不可行")
	}
	got := verifySolution(t, campaigns, zipCaps, res)
	// 最优：一个活动 A=59,B=1，另一个 A=60,B=0（或互换），总误差 20000。
	if got != 20000 {
		t.Fatalf("总误差 = %d，期望最小值 20000", got)
	}
}

// TestJointCapacityInfeasible 所有 ZIP 总上限足够、每个活动单独看也可行，
// 但共同可达的 ZIP 容量不足，整体不可行。
func TestJointCapacityInfeasible(t *testing.T) {
	campaigns := []Campaign{
		{ID: "c1", Budget: 5, Ideals: map[string]int64{"A": 50000}},
		{ID: "c2", Budget: 5, Ideals: map[string]int64{"A": 50000}},
	}
	// 总上限 8+5=13 >= 10，单个活动 5 <= 8 均可行；
	// 但两个活动都只能投 A，A 的容量 8 < 10。
	zipCaps := map[string]int64{"A": 8, "B": 5}
	if _, ok := Allocate(campaigns, zipCaps); ok {
		t.Fatal("联合容量不足，应返回不可行")
	}
}

// bruteForce 穷举所有 floor/ceil 组合，返回是否可行与最小总误差。
func bruteForce(campaigns []Campaign, zipCaps map[string]int64) (int64, bool) {
	type choice struct {
		ci       int
		zip      string
		floor, r int64
	}
	var choices []choice
	fixedPerCampaign := make([]int64, len(campaigns))
	fixedPerZip := make(map[string]int64)
	for ci, c := range campaigns {
		for zip, ideal := range c.Ideals {
			floor, r := ideal/quantum, ideal%quantum
			if r == 0 {
				fixedPerCampaign[ci] += floor
				fixedPerZip[zip] += floor
				continue
			}
			choices = append(choices, choice{ci: ci, zip: zip, floor: floor, r: r})
		}
	}
	best := int64(-1)
	n := len(choices)
	for mask := 0; mask < 1<<n; mask++ {
		perCampaign := append([]int64(nil), fixedPerCampaign...)
		perZip := make(map[string]int64, len(fixedPerZip))
		for z, v := range fixedPerZip {
			perZip[z] = v
		}
		var err int64
		for i, ch := range choices {
			amount := ch.floor
			e := ch.r
			if mask>>i&1 == 1 {
				amount++
				e = quantum - ch.r
			}
			perCampaign[ch.ci] += amount
			perZip[ch.zip] += amount
			err += e
		}
		valid := true
		for ci, c := range campaigns {
			if perCampaign[ci] != c.Budget {
				valid = false
			}
		}
		for z, v := range perZip {
			if v > zipCaps[z] {
				valid = false
			}
		}
		if valid && (best < 0 || err < best) {
			best = err
		}
	}
	return best, best >= 0
}

// genInstance 用固定种子生成微型随机输入，保证每个活动的理想值之和
// 恰好等于预算*10000。
func genInstance(rng *rand.Rand) ([]Campaign, map[string]int64) {
	nZips := 2 + rng.Intn(2)
	zips := []string{"A", "B", "C"}[:nZips]
	nCampaigns := 1 + rng.Intn(3)
	var campaigns []Campaign
	var totalBudget int64
	for i := 0; i < nCampaigns; i++ {
		budget := int64(rng.Intn(9)) // 0..8 美分
		totalBudget += budget
		// 随机非空目标子集
		var targets []string
		for _, z := range zips {
			if rng.Intn(2) == 0 {
				targets = append(targets, z)
			}
		}
		if len(targets) == 0 {
			targets = []string{zips[rng.Intn(nZips)]}
		}
		// 把 budget*10000 随机拆到各目标 ZIP 上
		ideals := make(map[string]int64, len(targets))
		remaining := budget * quantum
		for j, z := range targets {
			if j == len(targets)-1 {
				ideals[z] = remaining
			} else {
				v := rng.Int63n(remaining + 1)
				ideals[z] = v
				remaining -= v
			}
		}
		campaigns = append(campaigns, Campaign{
			ID:     string(rune('a' + i)),
			Budget: budget,
			Ideals: ideals,
		})
	}
	zipCaps := make(map[string]int64, nZips)
	for _, z := range zips {
		zipCaps[z] = int64(rng.Intn(int(totalBudget) + 3))
	}
	return campaigns, zipCaps
}

// TestAgainstBruteForce 对一批固定种子的微型输入，用穷举结果核对
// Allocate 的可行性判定与最小误差。
func TestAgainstBruteForce(t *testing.T) {
	seeds := []int64{1, 7, 42, 99, 12345, 20261004}
	feasibleCount := 0
	for _, seed := range seeds {
		rng := rand.New(rand.NewSource(seed))
		for iter := 0; iter < 200; iter++ {
			campaigns, zipCaps := genInstance(rng)
			wantErr, wantOK := bruteForce(campaigns, zipCaps)
			res, ok := Allocate(campaigns, zipCaps)
			if ok != wantOK {
				t.Fatalf("seed=%d iter=%d 可行性不一致：Allocate=%v 穷举=%v\n输入: %+v caps=%v",
					seed, iter, ok, wantOK, campaigns, zipCaps)
			}
			if !ok {
				continue
			}
			feasibleCount++
			got := verifySolution(t, campaigns, zipCaps, res)
			if got != wantErr {
				t.Fatalf("seed=%d iter=%d 最小误差不一致：Allocate=%d 穷举=%d\n输入: %+v caps=%v",
					seed, iter, got, wantErr, campaigns, zipCaps)
			}
		}
	}
	if feasibleCount == 0 {
		t.Fatal("随机用例全部不可行，测试无效")
	}
	t.Logf("对拍通过，其中可行用例 %d 个", feasibleCount)
}

// TestExactIntegerIdealsUnchanged 理想值已是整数美分时不得改变。
func TestExactIntegerIdealsUnchanged(t *testing.T) {
	campaigns := []Campaign{
		{ID: "c1", Budget: 10, Ideals: map[string]int64{"A": 70000, "B": 30000}},
		{ID: "c2", Budget: 4, Ideals: map[string]int64{"B": 40000}},
	}
	zipCaps := map[string]int64{"A": 100, "B": 100}
	res, ok := Allocate(campaigns, zipCaps)
	if !ok {
		t.Fatal("应为可行")
	}
	if res.TotalError != 0 {
		t.Fatalf("总误差应为 0，实际 %d", res.TotalError)
	}
	got := make(map[[2]string]int64)
	for _, a := range res.Allocations {
		got[[2]string{a.CampaignID, a.Zip}] = a.Amount
	}
	want := map[[2]string]int64{{"c1", "A"}: 7, {"c1", "B"}: 3, {"c2", "B"}: 4}
	if len(got) != len(want) {
		t.Fatalf("分配关系数 = %d，期望 %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("关系 %v 金额 = %d，期望 %d", k, got[k], v)
		}
	}
}

// 保证输出确定性：同一输入两次调用结果一致。
func TestDeterministic(t *testing.T) {
	campaigns, zipCaps := genInstance(rand.New(rand.NewSource(7)))
	r1, ok1 := Allocate(campaigns, zipCaps)
	r2, ok2 := Allocate(campaigns, zipCaps)
	if ok1 != ok2 {
		t.Fatal("两次调用可行性不一致")
	}
	if !ok1 {
		return
	}
	s1 := sortAllocations(r1.Allocations)
	s2 := sortAllocations(r2.Allocations)
	for i := range s1 {
		if s1[i] != s2[i] {
			t.Fatal("两次调用分配结果不一致")
		}
	}
}

func sortAllocations(in []Allocation) []Allocation {
	out := append([]Allocation(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CampaignID != out[j].CampaignID {
			return out[i].CampaignID < out[j].CampaignID
		}
		return out[i].Zip < out[j].Zip
	})
	return out
}
