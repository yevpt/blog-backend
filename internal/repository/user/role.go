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
