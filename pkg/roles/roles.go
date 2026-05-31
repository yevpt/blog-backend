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

// roleWeight 权重越小权限越高（Admin=1 可访问所有级别，Normal=3 不能访问 VIP=2 接口）
var roleWeight = map[string]int{
	AdminRole:  1,
	VipRole:    2,
	NormalRole: 3,
}

// Weight 返回角色权重，未知角色返回 999（兜底为最低权限，防止未知角色意外获得访问权）
func Weight(role string) int {
	// 在权重表中查找对应权重，未知角色返回 999 兜底（最低权限，防止未知角色意外获得访问权）
	if w, ok := roleWeight[role]; ok {
		return w
	}
	return 999
}

// HasPermission 用户持有的任意角色权重 ≤ minRole 权重时返回 true，即高权限角色自动覆盖低权限接口
func HasPermission(userRoles []string, minRole string) bool {
	required := Weight(minRole)
	// 遍历用户持有的所有角色，任意一个满足条件即有权限（OR 语义）
	for _, r := range userRoles {
		if Weight(r) <= required {
			return true
		}
	}
	return false
}
