package main

/*
 * @lc app=leetcode.cn id=494 lang=golang
 *
 * [494] 目标和
 */

// @lc code=start
func findTargetSumWays(nums []int, target int) int {
	sum := 0
	n := len(nums)
	for _, v := range nums {
		sum += v
	}

	if abs(target) > sum {
		return 0
	}
	if (sum+target)%2 == 1 {
		return 0
	}
	t := (sum + target) / 2

	dp := make([]int, t+1)
	dp[0] = 1
	for i := 0; i < n; i++ {
		for j := t; j >= nums[i]; j-- {
			dp[j] += dp[j-nums[i]]
		}
	}
	return dp[t]
}

func abs(x int) int {
	if x >= 0 {
		return x
	}
	return -x
}

// @lc code=end
