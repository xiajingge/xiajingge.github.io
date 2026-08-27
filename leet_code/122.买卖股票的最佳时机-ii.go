/*
 * @lc app=leetcode.cn id=122 lang=golang
 *
 * [122] 买卖股票的最佳时机 II
 */

// @lc code=start
func maxProfit(prices []int) int {
	n := len(prices)
	dp := make([][]int, n)
	for i, _ := range dp {
		dp[i] = make([]int, 2)
	}
	// 0是持有 1是不持有
	dp[0][0], dp[0][1] = -prices[0], 0

	for i := 1; i < n; i++ {
		dp[i][0] = max(dp[i-1][0], dp[i-1][1]-prices[i])
		dp[i][1] = max(dp[i-1][1], dp[i-1][0]+prices[i])
	}

	return dp[n-1][1]
}

// @lc code=end

