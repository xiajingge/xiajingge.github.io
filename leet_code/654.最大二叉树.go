package main

/*
 * @lc app=leetcode.cn id=654 lang=golang
 *
 * [654] 最大二叉树
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

func constructMaximumBinaryTree(nums []int) *TreeNode {
	stack := []*TreeNode{}
	for _, num := range nums {
		cur := &TreeNode{num, nil, nil}
		for len(stack) > 0 && stack[len(stack)-1].Val < num {
			cur.Left = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			stack[len(stack)-1].Right = cur
		}
		stack = append(stack, cur)
	}
	return stack[0]
}

// @lc code=end


1 2 3 4 5
0 1 2 3 4
	b b	