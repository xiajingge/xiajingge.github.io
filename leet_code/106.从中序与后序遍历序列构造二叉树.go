package main

/*
 * @lc app=leetcode.cn id=106 lang=golang
 *
 * [106] 从中序与后序遍历序列构造二叉树
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
func buildTree(inorder []int, postorder []int) *TreeNode {
	if len(inorder) == 0 || len(postorder) == 0 {
		return nil
	}
	root_val := postorder[len(postorder)-1]
	root := &TreeNode{root_val, nil, nil}

	root_index := 0
	for i := 0; i < len(inorder); i++ {
		if inorder[i] == root_val {
			root_index = i
			break
		}
	}

	inorder_left := inorder[:root_index]
	inorder_right := inorder[root_index+1:]

	postorder_left := postorder[:len(inorder_left)]
	postorder_right := postorder[len(inorder_left) : len(postorder)-1]

	root.Left = buildTree(inorder_left, postorder_left)
	root.Right = buildTree(inorder_right, postorder_right)
	return root
}

// @lc code=end
