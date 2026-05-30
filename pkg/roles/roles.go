package roles

// 角色名称常量，与数据库 roles 表中的 name 字段对应
const (
	AdminRole  = "ROLE_ADMIN"  // 管理员，权重最高
	VipRole    = "ROLE_VIP"    // VIP 用户
	NormalRole = "ROLE_NORMAL" // 普通用户，默认角色
)

// 角色 ID 常量，与数据库 roles 表中的 id 字段对应
const (
	AdminRoleId  = 1
	VipRoleId    = 2
	NormalRoleId = 3
)

// roleWeight 权重越小，权限越高。
// 访问控制逻辑：用户任意角色的权重 <= 所需角色权重，则放行。
// 例如：Admin(1) 可以访问 VIP(2) 和 Normal(3) 接口；Normal(3) 不能访问 VIP(2) 接口。
var roleWeight = map[string]int{
	AdminRole:  1,
	VipRole:    2,
	NormalRole: 3,
}

// Weight 返回角色对应的权重，角色不存在时返回最大值（最低权限）
func Weight(role string) int {
	if w, ok := roleWeight[role]; ok {
		return w
	}
	return 999
}

// HasPermission 检查用户角色列表中是否有权限访问 minRole 及以上级别
func HasPermission(userRoles []string, minRole string) bool {
	required := Weight(minRole)
	for _, r := range userRoles {
		if Weight(r) <= required {
			return true
		}
	}
	return false
}
