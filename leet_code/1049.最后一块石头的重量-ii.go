package main

/*
 * @lc app=leetcode.cn id=1049 lang=golang
 *
 * [1049] 最后一块石头的重量 II
 */

// @lc code=start
func lastStoneWeightII(stones []int) int {
	sum := 0
	n := len(stones)
	for _, v := range stones {
		sum += v
	}
	target := sum / 2
	dp := make([]int, target+1)

	for i := 0; i < n; i++ {
		for j := target; j >= stones[i]; j-- {
			dp[j] = max(dp[j], dp[j-stones[i]]+stones[i])
		}
	}

	return sum - 2*dp[target]
}

// @lc code=end
