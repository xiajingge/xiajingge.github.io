package main

/*
 * @lc app=leetcode.cn id=518 lang=golang
 *
 * [518] 零钱兑换 II
 */

// @lc code=start
func change(amount int, coins []int) int {
	n := len(coins)
	dp := make([]int, amount+1)
	dp[0] = 1
	for i := 0; i < n; i++ {
		for j := 1; j <= amount; j++ {
			if j >= coins[i] {
				dp[j] += dp[j-coins[i]]
			}
		}
	}

	return dp[amount]
}

// @lc code=end
