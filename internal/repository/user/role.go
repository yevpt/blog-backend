package user

import (
	"errors"

	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/roles"
)

// GrantVipRole 为目标用户追加 ROLE_VIP；已拥有时幂等成功。
func (r *userRepo) GrantVipRole(userID uint) error {
	var existing model.UserRole
	err := r.db.Where("user_id = ? AND role_id = ?", userID, roles.VipRoleId).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.Create(&model.UserRole{UserID: userID, RoleID: roles.VipRoleId}).Error
}

// RevokeVipRole 移除目标用户的 ROLE_VIP；本就不是 VIP 时幂等成功。
func (r *userRepo) RevokeVipRole(userID uint) error {
	return r.db.Where("user_id = ? AND role_id = ?", userID, roles.VipRoleId).
		Delete(&model.UserRole{}).Error
}

// SetStatus 更新用户账号状态（1=正常，0=已禁用）。
func (r *userRepo) SetStatus(userID uint, status uint8) error {
	return r.db.Model(&model.User{}).Where("id = ?", userID).Update("status", status).Error
}

// CountByRole 统计持有指定角色名称的用户数量，用于禁用前的“最后一个管理员”校验。
func (r *userRepo) CountByRole(roleName string) (int64, error) {
	var count int64
	err := r.db.Model(&model.UserRole{}).
		Joins("JOIN role ON role.id = user_role.role_id").
		Where("role.name = ?", roleName).
		Count(&count).Error
	return count, err
}
