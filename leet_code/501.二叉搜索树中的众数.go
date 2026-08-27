/*
 * @lc app=leetcode.cn id=501 lang=golang
 *
 * [501] 二叉搜索树中的众数
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

type Node struct {
	treenode *TreeNode
	handled  int
}

func findMode(root *TreeNode) []int {
	res := make([]int, 0)
	stack := make([]*Node, 0)
	stack = append(stack, &Node{root, 0})
	var pre *TreeNode
	count, maxcount := 1, 0
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if n.treenode == nil {
			continue
		}
		if n.handled == 0 {
			stack = append(stack, &Node{n.treenode.Right, 0})
			stack = append(stack, &Node{n.treenode, 1})
			stack = append(stack, &Node{n.treenode.Left, 0})
		} else {
			// 处理当前节点
			if pre == nil {
				pre = n.treenode
			} else {
				if pre.Val == n.treenode.Val {
					count++
				} else {
					count = 1
				}
				pre = n.treenode
			}
			// 更新res
			if count > maxcount {
				maxcount = count
				res = make([]int, 0)
				res = append(res, n.treenode.Val)
			} else if count == maxcount {
				res = append(res, n.treenode.Val)
			}
		}
	}
	return res
}

// @lc code=end

