package user

import (
	"github.com/vpt/blog-backend/internal/model"
)

// ListAllWithManagedAvatar 返回全部带有头像的用户，按 ID 升序。
func (r *userRepo) ListAllWithManagedAvatar() ([]model.User, error) {
	var users []model.User
	err := r.db.Model(&model.User{}).
		Where("avatar_url IS NOT NULL AND avatar_url != ''").
		Order("id ASC").
		Find(&users).Error
	return users, err
}
