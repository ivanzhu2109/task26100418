package autoreach

import (
	"math/rand"
	"testing"
)

// checkPlan 校验方案满足全部硬约束，并返回按方案算出的总误差。
func checkPlan(t *testing.T, campaigns []Campaign, zipCaps map[string]int64, plan Plan) int64 {
	t.Helper()
	zipUsed := make(map[string]int64)
	var totalErr int64
	for _, c := range campaigns {
		amts, ok := plan.Amounts[c.ID]
		if !ok {
			t.Fatalf("活动 %s 缺少分配结果", c.ID)
		}
		if len(amts) != len(c.Ideals) {
			t.Fatalf("活动 %s 的分配 ZIP 集合与目标集合不一致", c.ID)
		}
		var sum int64
		for z, ideal := range c.Ideals {
			a, ok := amts[z]
			if !ok {
				t.Fatalf("活动 %s 缺少 ZIP %s 的分配", c.ID, z)
			}
			fl, r := ideal/Scale, ideal%Scale
			if r == 0 && a != fl {
				t.Fatalf("活动 %s ZIP %s 理想值为整数美分，金额不得改变", c.ID, z)
			}
			if r > 0 && a != fl && a != fl+1 {
				t.Fatalf("活动 %s ZIP %s 金额 %d 不是理想值的向下/向上取整", c.ID, z, a)
			}
			d := a*Scale - ideal
			if d < 0 {
				d = -d
			}
			totalErr += d
			sum += a
			zipUsed[z] += a
		}
		if sum != c.Budget {
			t.Fatalf("活动 %s 分配总和 %d 不等于预算 %d", c.ID, sum, c.Budget)
		}
	}
	for z, used := range zipUsed {
		if used > zipCaps[z] {
			t.Fatalf("ZIP %s 使用 %d 超出上限 %d", z, used, zipCaps[z])
		}
	}
	if plan.TotalError != totalErr {
		t.Fatalf("返回的总误差 %d 与方案实际误差 %d 不符", plan.TotalError, totalErr)
	}
	return totalErr
}

// bruteForce 穷举所有取整组合，返回最小总误差与可行性，作为测试基准。
func bruteForce(campaigns []Campaign, zipCaps map[string]int64) (int64, bool) {
	type option struct {
		amounts map[string]int64
		err     int64
	}
	perCampaign := make([][]option, len(campaigns))
	for i, c := range campaigns {
		type rel struct {
			zip string
			fl  int64
			r   int64
		}
		var fixed map[string]int64 = make(map[string]int64)
		var fracs []rel
		var flSum int64
		for z, v := range c.Ideals {
			fl, r := v/Scale, v%Scale
			fixed[z] = fl
			flSum += fl
			if r > 0 {
				fracs = append(fracs, rel{z, fl, r})
			}
		}
		k := c.Budget - flSum
		if k < 0 || k > int64(len(fracs)) {
			return 0, false
		}
		var opts []option
		var choose func(start int, left int64, picked []bool)
		picked := make([]bool, len(fracs))
		choose = func(start int, left int64, pickedCnt []bool) {
			if left == 0 {
				amts := make(map[string]int64, len(fixed))
				for z, v := range fixed {
					amts[z] = v
				}
				var err int64
				for j, rl := range fracs {
					if pickedCnt[j] {
						amts[rl.zip]++
						err += Scale - rl.r
					} else {
						err += rl.r
					}
				}
				opts = append(opts, option{amts, err})
				return
			}
			for j := start; j <= len(fracs)-int(left); j++ {
				picked[j] = true
				choose(j+1, left-1, picked)
				picked[j] = false
			}
		}
		choose(0, k, picked)
		perCampaign[i] = opts
	}
	best := int64(-1)
	zipUsed := make(map[string]int64)
	var dfs func(i int, errAcc int64)
	dfs = func(i int, errAcc int64) {
		if i == len(perCampaign) {
			for z, u := range zipUsed {
				if u > zipCaps[z] {
					return
				}
			}
			if best < 0 || errAcc < best {
				best = errAcc
			}
			return
		}
		for _, opt := range perCampaign[i] {
			ok := true
			for z, a := range opt.amounts {
				zipUsed[z] += a
				if zipUsed[z] > zipCaps[z] {
					ok = false // 提前剪枝，仍继续累加其余 ZIP 后回退
				}
			}
			if ok {
				dfs(i+1, errAcc+opt.err)
			}
			for z, a := range opt.amounts {
				zipUsed[z] -= a
			}
		}
	}
	dfs(0, 0)
	if best < 0 {
		return 0, false
	}
	return best, true
}

// TestSharedZipRequiresReallocation 两个活动都偏向同一 ZIP，
// 各自取最接近理想值的舍入会超出共享 ZIP 上限，必须重新分配。
func TestSharedZipRequiresReallocation(t *testing.T) {
	campaigns := []Campaign{
		{ID: "A", Budget: 2, Ideals: map[string]int64{"Z1": 16000, "Z2": 4000}},
		{ID: "B", Budget: 2, Ideals: map[string]int64{"Z1": 17000, "Z2": 3000}},
	}
	// Z1 上限 3：A、B 都想把 Z1 进位到 2 美分，合计 4 超限。
	zipCaps := map[string]int64{"Z1": 3, "Z2": 10}
	plan, ok := Allocate(campaigns, zipCaps)
	if !ok {
		t.Fatal("应当可行")
	}
	got := checkPlan(t, campaigns, zipCaps, plan)
	// 最优：B 在 Z1 进位（误差 3000+3000），A 在 Z2 进位（误差 6000+6000）。
	const want = 18000
	if got != want {
		t.Fatalf("总误差 = %d，期望 %d", got, want)
	}
	if plan.Amounts["A"]["Z1"] != 1 || plan.Amounts["A"]["Z2"] != 1 ||
		plan.Amounts["B"]["Z1"] != 2 || plan.Amounts["B"]["Z2"] != 0 {
		t.Fatalf("分配结果不符合最优方案: %+v", plan.Amounts)
	}
}

// TestBruteForce 对一批固定种子的微型输入，用穷举结果核对可行性与最小误差。
func TestBruteForce(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		nz := 2 + rng.Intn(2)
		zips := []string{"Z1", "Z2", "Z3"}[:nz]
		nc := 2 + rng.Intn(2)
		var campaigns []Campaign
		for i := 0; i < nc; i++ {
			budget := 1 + int64(rng.Intn(6))
			var targets []string
			for _, z := range zips {
				if rng.Intn(10) < 7 {
					targets = append(targets, z)
				}
			}
			if len(targets) == 0 {
				targets = []string{zips[rng.Intn(nz)]}
			}
			ideals := make(map[string]int64, len(targets))
			remaining := budget * Scale
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
				ID:     string(rune('A' + i)),
				Budget: budget,
				Ideals: ideals,
			})
		}
		zipCaps := make(map[string]int64, nz)
		for _, z := range zips {
			zipCaps[z] = int64(rng.Intn(8))
		}

		wantErr, wantOK := bruteForce(campaigns, zipCaps)
		plan, ok := Allocate(campaigns, zipCaps)
		if ok != wantOK {
			t.Fatalf("seed=%d 可行性不一致: Allocate=%v 穷举=%v", seed, ok, wantOK)
		}
		if !ok {
			continue
		}
		got := checkPlan(t, campaigns, zipCaps, plan)
		if got != wantErr {
			t.Fatalf("seed=%d 最小误差不一致: Allocate=%d 穷举=%d", seed, got, wantErr)
		}
	}
}

// TestJointlyInfeasible 总上限足够、每个活动单独可行，
// 但共同可达的 ZIP 容量不足，整体不可行。
func TestJointlyInfeasible(t *testing.T) {
	campaigns := []Campaign{
		// A 只能投 Z1，预算 5 美分。
		{ID: "A", Budget: 5, Ideals: map[string]int64{"Z1": 50000}},
		// B 预算 5 美分，理想值 Z1=4.5、Z2=0.5，Z1 上至少占 4 美分。
		{ID: "B", Budget: 5, Ideals: map[string]int64{"Z1": 45000, "Z2": 5000}},
	}
	// 总上限 10 等于总预算 10，且每个活动单独看都放得下；
	// 但 Z1 必被占用至少 5+4=9 > 5，Z2 又无法承接 A 的预算。
	zipCaps := map[string]int64{"Z1": 5, "Z2": 5}
	if _, ok := Allocate(campaigns, zipCaps); ok {
		t.Fatal("应当不可行")
	}
	for _, c := range campaigns {
		if _, ok := Allocate([]Campaign{c}, zipCaps); !ok {
			t.Fatalf("活动 %s 单独应当可行", c.ID)
		}
	}
}
