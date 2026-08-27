package main

/*
 * @lc app=leetcode.cn id=474 lang=golang
 *
 * [474] 一和零
 */

// @lc code=start
func findMaxForm(strs []string, m int, n int) int {
	dp := make([][]int, m+1)
	for i, _ := range dp {
		dp[i] = make([]int, n+1)
	}

	// 遍历物品
	for i := 0; i < len(strs); i++ {
		x, y := 0, 0
		for _, v := range strs[i] {
			if v == '0' {
				x++
			}
			if v == '1' {
				y++
			}
		}

		// 遍历背包容量
		for j := m; j >= x; j-- {
			for k := n; k >= y; k-- {
				dp[j][k] = max(dp[j][k], dp[j-x][k-y]+1)
			}
		}
	}
	return dp[m][n]
}

// @lc code=end
