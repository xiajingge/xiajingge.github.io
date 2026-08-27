package main

func rob(root *TreeNode) int {
	if root == nil {
		return 0
	}

	res := robTree(root)
	return max(res[0], res[1])
}

func robTree(cur *TreeNode) []int {
	if cur == nil {
		return []int{0, 0}
	}

	left := robTree(cur.Left)
	right := robTree(cur.Right)

	// 偷根节点
	robroot := cur.Val + left[0] + right[0]

	// 不偷根节点
	notrobroot := max(left[0], left[1]) + max(right[0], right[1])

	return []int{notrobroot, robroot}
}
