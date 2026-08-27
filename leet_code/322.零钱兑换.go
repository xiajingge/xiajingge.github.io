package main

import "math"

/*
 * @lc app=leetcode.cn id=322 lang=golang
 *
 * [322] 零钱兑换
 */

// @lc code=start
func coinChange(coins []int, amount int) int {
	n := len(coins)
	dp := make([]int, amount+1)

	for i, _ := range dp {
		if i == 0 {
			dp[i] = 0
		} else {
			dp[i] = math.MaxInt32
		}
	}

	for j := 1; j <= amount; j++ {
		for i := 0; i < n; i++ {
			if j >= coins[i] {
				dp[j] = min(dp[j], dp[j-coins[i]]+1)
			}
		}
	}
	if dp[amount] == math.MaxInt32 {
		return -1
	}
	return dp[amount]
}

// @lc code=end
