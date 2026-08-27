package main

/*
 * @lc app=leetcode.cn id=188 lang=golang
 *
 * [188] 买卖股票的最佳时机 IV
 */

// @lc code=start
func maxProfit(k int, prices []int) int {
	n := len(prices)
	dp := make([][]int, n)
	for i, _ := range dp {
		dp[i] = make([]int, 2*k+1)
	}

	// 只有奇数位置是-prices[0]
	for i := 1; i <= 2*k; i += 2 {
		dp[0][i] = -prices[0]
	}

	for i := 1; i < n; i++ {
		for j := 1; j < 2*k+1; j += 2 {
			dp[i][j] = max(dp[i-1][j-1]-prices[i], dp[i-1][j])
			dp[i][j+1] = max(dp[i-1][j]+prices[i], dp[i-1][j+1])
		}
	}

	return dp[n-1][2*k]
}

// @lc code=end
