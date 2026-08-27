package main

/*
 * @lc app=leetcode.cn id=123 lang=golang
 *
 * [123] 买卖股票的最佳时机 III
 */

// @lc code=start
func maxProfit(prices []int) int {
	n := len(prices)
	dp := make([][][]int, n)
	for i, _ := range dp {
		dp[i] = make([][]int, 2) // 0持有或者1不持有
		for j, _ := range dp[i] {
			dp[i][j] = make([]int, 3) // 1第一次持有  2 第二次持有
		}
	}

	dp[0][0][1] = -prices[0]

	dp[0][1][1] = 0

	dp[0][0][2] = -prices[0]

	dp[0][1][2] = 0

	for i := 1; i < n; i++ {
		// 第一次持有
		dp[i][0][1] = max(-prices[i], dp[i-1][0][1])
		// 第一次不持有
		dp[i][1][1] = max(dp[i-1][0][1]+prices[i], dp[i-1][1][1])
		// 第二次持有
		dp[i][0][2] = max(dp[i-1][1][1]-prices[i], dp[i-1][0][2])
		// 第二次不持有
		dp[i][1][2] = max(dp[i-1][0][2]+prices[i], dp[i-1][1][2])
	}

	return dp[n-1][1][2]
}

// @lc code=end
