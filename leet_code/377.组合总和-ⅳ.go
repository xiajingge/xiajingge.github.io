package main

/*
 * @lc app=leetcode.cn id=377 lang=golang
 *
 * [377] 组合总和 Ⅳ
 */

// @lc code=start
func combinationSum4(nums []int, target int) int {
	n := len(nums)
	dp := make([]int, target+1)

	dp[0] = 1

	for j := 1; j <= target; j++ {
		for i := 0; i < n; i++ {
			if j >= nums[i] {
				dp[j] += dp[j-nums[i]]
			}
		}
	}
	return dp[target]

}

// @lc code=end
