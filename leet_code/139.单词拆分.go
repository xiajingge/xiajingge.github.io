package main

/*
 * @lc app=leetcode.cn id=139 lang=golang
 *
 * [139] 单词拆分
 */

// @lc code=start
func wordBreak(s string, wordDict []string) bool {
	wordSet := make(map[string]bool)
	for _, v := range wordDict {
		wordSet[v] = true
	}

	dp := make(map[int]bool)

	dp[0] = true

	for j := 1; j <= len(s); j++ {
		for i := 0; i < j; i++ {
			if dp[i] && wordSet[s[i:j]] {
				dp[j] = true
			}
		}
	}
	return dp[len(s)]
}

// @lc code=end
