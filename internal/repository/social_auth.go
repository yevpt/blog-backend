package repository

import (
	"errors"

	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
)

// SocialBinding 是用户与第三方平台绑定关系的轻量投影。
type SocialBinding struct {
	AuthID       uint   `gorm:"column:social_user_auth_id"`
	SocialUserID uint   `gorm:"column:social_user_id"`
	Source       string `gorm:"column:source"`
}

// SocialAuthRepository 封装第三方身份与本站用户的绑定数据访问。
type SocialAuthRepository interface {
	// FindSocialUser 按平台和第三方稳定 ID 查询第三方身份。
	FindSocialUser(source string, uuid string) (*model.SocialUser, error)
	// FindUserBySocialUserID 查询第三方身份已绑定的本站用户。
	FindUserBySocialUserID(socialUserID uint) (*model.User, error)
	// CreateUserWithSocialAuth 在事务中创建本站用户、默认角色、第三方身份和绑定关系。
	CreateUserWithSocialAuth(user *model.User, roleID uint, socialUser *model.SocialUser) error
	// BindExistingUser 把第三方身份绑定到已有本站用户。
	BindExistingUser(userID uint, socialUser *model.SocialUser) error
	// FindBindingByUserAndSource 查询用户在某个平台上的绑定关系。
	FindBindingByUserAndSource(userID uint, source string) (*SocialBinding, error)
	// ListBindings 查询用户所有第三方绑定。
	ListBindings(userID uint) ([]SocialBinding, error)
	// CountBindings 统计用户当前有效的第三方绑定数量。
	CountBindings(userID uint) (int64, error)
	// Unbind 软删除用户与指定平台的绑定关系。
	Unbind(userID uint, source string) error
}

type socialAuthRepo struct {
	db *gorm.DB
}

func NewSocialAuthRepository(db *gorm.DB) SocialAuthRepository {
	return &socialAuthRepo{db: db}
}

func (r *socialAuthRepo) FindSocialUser(source string, uuid string) (*model.SocialUser, error) {
	var socialUser model.SocialUser
	// source + uuid 是第三方身份的唯一业务键。
	err := r.db.Where("source = ? AND uuid = ?", source, uuid).First(&socialUser).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &socialUser, err
}

func (r *socialAuthRepo) FindUserBySocialUserID(socialUserID uint) (*model.User, error) {
	var auth model.SocialUserAuth
	// 先查绑定关系，再按 user_id 查用户，避免 repository 返回跨层 DTO。
	err := r.db.Where("social_user_id = ?", socialUserID).First(&auth).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var user model.User
	err = r.db.First(&user, auth.UserID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *socialAuthRepo) CreateUserWithSocialAuth(user *model.User, roleID uint, socialUser *model.SocialUser) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 先创建本站用户，拿到自增 user_id。
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		// OAuth 新用户默认授予普通用户角色。
		if err := tx.Create(&model.UserRole{UserID: user.ID, RoleID: roleID}).Error; err != nil {
			return err
		}
		// 第三方身份单独落表，后续同一身份再次登录可直接复用。
		if socialUser.ID == 0 {
			if err := tx.Create(socialUser).Error; err != nil {
				return err
			}
		}
		// 最后写绑定关系，保证 user 和 social_user 都已经有主键。
		return tx.Create(&model.SocialUserAuth{
			UserID:       user.ID,
			SocialUserID: socialUser.ID,
		}).Error
	})
}

func (r *socialAuthRepo) BindExistingUser(userID uint, socialUser *model.SocialUser) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 新第三方身份先写 social_user；已有身份直接复用 ID。
		if socialUser.ID == 0 {
			if err := tx.Create(socialUser).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.SocialUserAuth{
			UserID:       userID,
			SocialUserID: socialUser.ID,
		}).Error
	})
}

func (r *socialAuthRepo) FindBindingByUserAndSource(userID uint, source string) (*SocialBinding, error) {
	var binding SocialBinding
	err := bindingQuery(r.db).
		Where("social_user_auth.user_id = ? AND social_user.source = ?", userID, source).
		Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &binding, err
}

func (r *socialAuthRepo) ListBindings(userID uint) ([]SocialBinding, error) {
	var bindings []SocialBinding
	err := bindingQuery(r.db).
		Where("social_user_auth.user_id = ?", userID).
		Order("social_user.source ASC").
		Find(&bindings).Error
	return bindings, err
}

func (r *socialAuthRepo) CountBindings(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.SocialUserAuth{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return count, err
}

func (r *socialAuthRepo) Unbind(userID uint, source string) error {
	binding, err := r.FindBindingByUserAndSource(userID, source)
	if err != nil || binding == nil {
		return err
	}
	// 只软删除绑定关系，保留 social_user 便于审计和后续重新绑定。
	return r.db.Delete(&model.SocialUserAuth{Base: model.Base{ID: binding.AuthID}}).Error
}

func bindingQuery(db *gorm.DB) *gorm.DB {
	return db.Table("social_user_auth").
		Select("social_user_auth.id AS social_user_auth_id, social_user.id AS social_user_id, social_user.source").
		Joins("JOIN social_user ON social_user.id = social_user_auth.social_user_id").
		Where("social_user_auth.deleted_at IS NULL").
		Where("social_user.deleted_at IS NULL")
}
