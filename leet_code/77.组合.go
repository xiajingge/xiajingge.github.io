/*
 * @lc app=leetcode.cn id=77 lang=golang
 *
 * [77] 组合
 */

// @lc code=start
func combine(n int, k int) [][]int {
	res := make([][]int, 0)
	path := make([]int, 0, k)
	var backtracking func(n int, k int, startindex int)
	backtracking = func(n int, k int, startindex int) {
		if len(path) == k {
			tmp := make([]int, k)
			copy(tmp, path)
			res = append(res, tmp)
			return
		}
		for i := startindex; i <= (n - (k - len(path)) + 1); i++ {
			path = append(path, i)
			backtracking(n, k, i+1)
			path = path[:len(path)-1]
		}
	}

	backtracking(n, k, 1)
	return res
}

// @lc code=end

