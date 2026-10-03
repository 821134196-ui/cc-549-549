package main

// 所有金额均为 int64 分。版税总额按比例算出后做四舍五入（银行外常用做法），
// 再按作者 bps 拆分；由于版税基数也是分（最小货币单位），余数最多几分钱，
// 采用最大余数法把零头分给第一作者，保证分配合计与总额分毫不差。

func divBps(amount int64, bps int64) int64 {
	// amount * bps / 10000，四舍五入到整数分
	num := amount * bps
	q := num / 10000
	r := num % 10000
	if r < 0 {
		r = -r
		if r*2 >= 10000 {
			q-- // 负数半分向上（绝对值），与正向对称
		}
		return q
	}
	if r*2 >= 10000 {
		q++
	}
	return q
}

// splitByBps 把 total 按 bps 列表拆分。bps 合计必须为 10000。
// 处理正负两种 total（退货为负），分配合计恒等于 total。
func splitByBps(total int64, bpsList []int64) []int64 {
	n := len(bpsList)
	out := make([]int64, n)
	if n == 0 {
		return out
	}
	remaining := total
	// 先按截断法分配
	raw := make([]int64, n)
	type rem struct {
		i  int
		rv int64
	}
	positive := total >= 0
	_ = positive
	for i, bps := range bpsList {
		v := total * bps / 10000
		raw[i] = v
		remaining -= v
	}
	copy(out, raw)
	if remaining == 0 {
		return out
	}
	// 把零头（|remaining| 分）按 bps 大小顺序逐分给出，确定性、可复现
	type idx struct {
		i   int
		bps int64
	}
	order := make([]idx, n)
	for i, bps := range bpsList {
		order[i] = idx{i, bps}
	}
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			if order[b].bps > order[a].bps || (order[b].bps == order[a].bps && order[b].i < order[a].i) {
				order[a], order[b] = order[b], order[a]
			}
		}
	}
	step := int64(1)
	if remaining < 0 {
		step = -1
	}
	k := 0
	for remaining != 0 {
		out[order[k%n].i] += step
		remaining -= step
		k++
	}
	return out
}
