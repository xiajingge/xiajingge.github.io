package main

import "math"

/*
 * @lc app=leetcode.cn id=279 lang=golang
 *
 * [279] 完全平方数
 */

// @lc code=start
func numSquares(n int) int {
	dp := make([]int, n+1)
	for i, _ := range dp {
		if i == 0 {
			continue
		} else {
			dp[i] = math.MaxInt32
		}
	}

	for i := 1; i*i <= n; i++ {
		for j := i * i; j <= n; j++ {
			dp[j] = min(dp[j], dp[j-i*i]+1)
		}
	}

	if dp[n] == math.MaxInt32 {
		return 0
	}
	return dp[n]
}

// @lc code=end
