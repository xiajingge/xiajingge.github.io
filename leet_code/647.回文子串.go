package main

/*
 * @lc app=leetcode.cn id=647 lang=golang
 *
 * [647] 回文子串
 */

// @lc code=start
func countSubstrings(s string) int {
	n := len(s)
	dp := make([][]bool, n)
	for i, _ := range dp {
		dp[i] = make([]bool, n)
	}

	res := 0

	for i := 0; i < n; i++ {
		dp[i][i] = true
		res++
	}

	for i := n - 1; i >= 0; i-- {
		for j := i + 1; j < n; j++ {
			if s[i] == s[j] {
				if j-i == 1 {
					dp[i][j] = true
					res++
				} else if dp[i+1][j-1] == true {
					dp[i][j] = true
					res++
				}
			}
		}
	}
	return res
}

// @lc code=end
