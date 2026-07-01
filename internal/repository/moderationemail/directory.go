package moderationemail

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// LoadAdminRecipient 只返回指定的活跃、已验证主邮箱管理员。
func (r *repository) LoadAdminRecipient(ctx context.Context, userID uint) (AdminRecipient, error) {
	var recipient AdminRecipient
	err := r.db.WithContext(ctx).Table("user").
		Select("user.id AS user_id,user.email").
		Joins("JOIN user_role ON user_role.user_id = user.id").
		Joins("JOIN role ON role.id = user_role.role_id").
		Where("user.id = ?", userID).
		Where("user.status = ?", 1).
		Where("user.email IS NOT NULL AND user.email <> ''").
		Where("user.email_verified_at IS NOT NULL").
		Where("role.name = ?", "admin").
		Order("user.id").
		Limit(1).
		Take(&recipient).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AdminRecipient{}, ErrRecipientUnavailable
	}
	if err != nil {
		return AdminRecipient{}, fmt.Errorf("load moderation email recipient: %w", err)
	}
	return recipient, nil
}
