package main

/*
 * @lc app=leetcode.cn id=113 lang=golang
 *
 * [113] 路径总和 II
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
func pathSum(root *TreeNode, targetSum int) [][]int {
	if root == nil {
		return nil
	}
	res := make([][]int, 0)
	path := make([]int, 0)
	var dfs func(node *TreeNode, sum int, path []int)
	dfs = func(node *TreeNode, sum int, path []int) {
		if node == nil {
			return
		}
		sum += node.Val
		path = append(path, node.Val)
		if node.Left == nil && node.Right == nil {
			if sum == targetSum {
				tmp := make([]int, len(path))
				copy(tmp, path)
				res = append(res, tmp)
			}
		}
		dfs(node.Left, sum, path)
		dfs(node.Right, sum, path)
	}
	dfs(root, 0, path)
	return res
}

// @lc code=end
