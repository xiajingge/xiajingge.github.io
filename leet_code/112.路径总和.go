package main

/*
 * @lc app=leetcode.cn id=112 lang=golang
 *
 * [112] 路径总和
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
func hasPathSum(root *TreeNode, targetSum int) bool {
	if root == nil {
		return false
	}
	res := false
	var dfs func(node *TreeNode, sum int)
	dfs = func(node *TreeNode, sum int) {
		if node == nil {
			return
		}
		sum += node.Val
		if node.Left == nil && node.Right == nil {
			// 说明是叶子
			if sum == targetSum {
				res = true
			}
		}
		if node.Left != nil {
			dfs(node.Left, sum)
		}
		if node.Right != nil {
			dfs(node.Right, sum)
		}
	}
	dfs(root, 0)
	return res
}

// @lc code=end
