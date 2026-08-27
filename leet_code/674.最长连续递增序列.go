package main

/*
 * @lc app=leetcode.cn id=674 lang=golang
 *
 * [674] 最长连续递增序列
 */

// @lc code=start
func findLengthOfLCIS(nums []int) int {
	n := len(nums)
	dp := make([]int, n)
	dp[0] = 1

	res := 1
	for i := 1; i < n; i++ {
		if nums[i] > nums[i-1] {
			dp[i] = dp[i-1] + 1
		} else {
			dp[i] = 1
		}

		if res < dp[i] {
			res = dp[i]
		}
	}
	return res
}

// @lc code=end
