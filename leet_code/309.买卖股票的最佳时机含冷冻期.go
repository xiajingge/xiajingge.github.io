package main

/*
 * @lc app=leetcode.cn id=309 lang=golang
 *
 * [309] 买卖股票的最佳时机含冷冻期
 */

// @lc code=start
func maxProfit(prices []int) int {
	n := len(prices)
	dp := make([][]int, n)
	for i, _ := range dp {
		dp[i] = make([]int, 4)
	}
	dp[0][0] = -prices[0]

	dp[0][1] = 0

	dp[0][2] = 0

	dp[0][3] = 0

	for i := 1; i < n; i++ {
		dp[i][0] = max(dp[i-1][0], dp[i-1][1]-prices[i], dp[i-1][3]-prices[i])

		dp[i][1] = max(dp[i-1][1], dp[i-1][3])

		dp[i][2] = dp[i-1][0] + prices[i]

		dp[i][3] = dp[i-1][2]
	}
	return max(dp[n-1][1], dp[n-1][2], dp[n-1][3])
}

// @lc code=end
