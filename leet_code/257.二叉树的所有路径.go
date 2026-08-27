package main

import "strconv"

/*
 * @lc app=leetcode.cn id=257 lang=golang
 *
 * [257] 二叉树的所有路径
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
func binaryTreePaths(root *TreeNode) []string {
	res := make([]string, 0)
	s := ""
	var dfs func(node *TreeNode, s string)
	dfs = func(node *TreeNode, s string) {
		if node.Left == nil && node.Right == nil {
			// 遇到了叶子节点，开始记录路径
			s += strconv.Itoa(node.Val)
			res = append(res, s)
		}
		s += strconv.Itoa(node.Val)
		s += "->"
		if node.Left != nil {
			dfs(node.Left, s)
		}
		if node.Right != nil {
			dfs(node.Right, s)
		}
	}
	dfs(root, s)
	return res
}

// @lc code=end
