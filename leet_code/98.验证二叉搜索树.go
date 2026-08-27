import "math"

/*
 * @lc app=leetcode.cn id=98 lang=golang
 *
 * [98] 验证二叉搜索树
 */

// @lc code=start
/**
 * Definition for a binary tree node.
 * type TreeNode struct {
 *     Val int
 *     Left *TreeNode
 *     Right *TreeNode
 * }
 */
func isValidBST(root *TreeNode) bool {
	res := true
	pre := math.MinInt64

	var dfs func(root *TreeNode)
	dfs = func(root *TreeNode) {
		if root == nil || res == false {
			return
		}
		dfs(root.Left)
		if pre < root.Val {
			pre = root.Val
		} else {
			res = false
			return
		}
		dfs(root.Right)
	}
	dfs(root)
	return res
}

// @lc code=end

