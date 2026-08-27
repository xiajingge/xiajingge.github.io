package main

/*
 * @lc app=leetcode.cn id=300 lang=golang
 *
 * [300] 最长递增子序列
 */

// @lc code=start
func lengthOfLIS(nums []int) int {
	n := len(nums)
	dp := make([]int, n)
	for i, _ := range dp {
		dp[i] = 1
	}

	res := 1

	for i := 1; i < n; i++ {
		for j := 0; j < i; j++ {
			if nums[i] > nums[j] {
				dp[i] = max(dp[j]+1, dp[i])
			}
		}
		if res < dp[i] {
			res = dp[i]
		}
	}

	return res
}

// @lc code=end
