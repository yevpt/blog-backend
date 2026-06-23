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

// maxRootLabelRunes 根对象快照的最大展示长度，超出以省略号收尾。
const maxRootLabelRunes = 60

// maxRootExcerptRunes 文章正文摘录的最大展示长度，超出以省略号收尾。
// 取 200 字作为站内通知正文预览，与标题级 root_label 区分。
const maxRootExcerptRunes = 200

// RootSnapshotOf 取根对象的展示快照文本：文章返回标题，碎语返回内容摘要，
// 其余类型（留言板等）留空。用于通知邮件正文中标识「哪篇文章/哪条碎语」。
// 对象不存在或被删除时返回空串，不视作错误，避免阻断邮件发送。
func (d *Directory) RootSnapshotOf(ctx context.Context, objectType string, objectID uint) (string, error) {
	switch objectType {
	case "article":
		return d.scalarString(ctx, &model.Article{}, "title", objectID, maxRootLabelRunes)
	case "moment":
		return d.scalarString(ctx, &model.Moment{}, "content", objectID, maxRootLabelRunes)
	default:
		return "", nil
	}
}

// RootExcerptOf 取文章正文摘录，仅 article 类型有效，其余返回空串。
// 用于站内通知列表展示正文预览；文章不存在或被删除时返回空串，不视作错误。
func (d *Directory) RootExcerptOf(ctx context.Context, objectType string, objectID uint) (string, error) {
	switch objectType {
	case "article":
		return d.scalarString(ctx, &model.Article{}, "content", objectID, maxRootExcerptRunes)
	default:
		return "", nil
	}
}

// scalarString 取某表指定行的字符串列，不存在返回空串，并对结果按 rune 截断。
func (d *Directory) scalarString(ctx context.Context, table any, column string, id uint, maxRunes int) (string, error) {
	var value string
	err := d.db.WithContext(ctx).Model(table).
		Select(column).
		Where("id = ?", id).
		Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return truncateRunes(value, maxRunes), nil
}

// truncateRunes 按 rune 截断字符串，超出 max 时尾部以「…」收尾。
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
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
