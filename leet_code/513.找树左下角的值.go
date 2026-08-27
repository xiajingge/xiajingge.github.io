package main

import "container/list"

/*
 * @lc app=leetcode.cn id=513 lang=golang
 *
 * [513] 找树左下角的值
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
func findBottomLeftValue(root *TreeNode) int {
	res := 0
	queue := list.New()
	queue.PushBack(root)
	for queue.Len() > 0 {
		length := queue.Len()
		for i := 0; i < length; i++ {
			n := queue.Remove(queue.Front()).(*TreeNode)
			if i == 0 {
				// 这说明是每层的第一个
				res = n.Val
			}
			if n.Left != nil {
				queue.PushBack(n.Left)
			}
			if n.Right != nil {
				queue.PushBack(n.Right)
			}
		}
	}
	return res
}

// @lc code=end
