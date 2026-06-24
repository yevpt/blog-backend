package userrole

// Lookup 批量查询用户角色的最小接口，避免 service 层依赖完整 UserRepository。
type Lookup interface {
	FindRolesByUserIDs(userIDs []uint) (map[uint][]string, error)
}

// LookupByUserIDs 去重后批量查询用户角色。
func LookupByUserIDs(repo Lookup, userIDs []uint) (map[uint][]string, error) {
	unique := uniqueUints(userIDs)
	if len(unique) == 0 {
		return map[uint][]string{}, nil
	}
	return repo.FindRolesByUserIDs(unique)
}

// ForUser 从角色字典中取出指定用户的角色列表，无记录时返回空切片。
func ForUser(rolesMap map[uint][]string, userID uint) []string {
	if roles := rolesMap[userID]; roles != nil {
		return roles
	}
	return []string{}
}

func uniqueUints(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
