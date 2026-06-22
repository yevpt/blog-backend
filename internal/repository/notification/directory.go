package notification

import (
	"context"
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// Directory 是通知分发读侧适配器：解析对象归属、接收人邮件资料与用户角色。
// 它跨业务表只读查询，供 dispatcher/planner/sender 使用，不参与写入。
type Directory struct {
	db *gorm.DB
}

// NewDirectory 创建分发读侧适配器。
func NewDirectory(db *gorm.DB) *Directory {
	return &Directory{db: db}
}

// OwnerOf 解析某对象的归属用户：文章/碎语取作者，留言取板主。
func (d *Directory) OwnerOf(ctx context.Context, objectType string, objectID uint) (uint, bool, error) {
	switch objectType {
	case "article":
		return d.scalarOwner(ctx, &model.Article{}, "user_id", objectID)
	case "moment":
		return d.scalarOwner(ctx, &model.Moment{}, "user_id", objectID)
	case "guestbook":
		return d.scalarOwner(ctx, &model.Guestbook{}, "owner_user_id", objectID)
	default:
		return 0, false, nil
	}
}

// scalarOwner 取某表指定行的归属用户列，不存在返回 found=false。
func (d *Directory) scalarOwner(ctx context.Context, table any, column string, id uint) (uint, bool, error) {
	var ownerID uint
	err := d.db.WithContext(ctx).Model(table).
		Select(column).
		Where("id = ?", id).
		Take(&ownerID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return ownerID, true, nil
}

// MailProfile 返回用户邮箱与总邮件开关（user_setting.receive_mail）。
// 邮箱缺失或未配置邮件开关时分别以空串、false 兜底。
func (d *Directory) MailProfile(ctx context.Context, userID uint) (string, bool, error) {
	var user model.User
	err := d.db.WithContext(ctx).Select("id", "email").Take(&user, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	email := ""
	if user.Email != nil {
		email = *user.Email
	}

	var setting model.UserSetting
	err = d.db.WithContext(ctx).Select("user_id", "receive_mail").
		Where("user_id = ?", userID).Take(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 无设置行时默认接收邮件，与 user_setting 默认值一致。
		return email, true, nil
	}
	if err != nil {
		return "", false, err
	}
	return email, setting.ReceiveMail, nil
}

// Roles 返回用户的角色名列表。
func (d *Directory) Roles(ctx context.Context, userID uint) ([]string, error) {
	var roles []string
	err := d.db.WithContext(ctx).Model(&model.UserRole{}).
		Joins("JOIN role ON role.id = user_role.role_id").
		Where("user_role.user_id = ?", userID).
		Pluck("role.name", &roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}
