package user

import (
	"context"
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// AuthenticationRepository 是认证链路使用的最小用户仓储，所有数据库操作都接受请求 context。
type AuthenticationRepository interface {
	FindByIdentifier(ctx context.Context, identifier string) (*model.User, error)
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByNickname(ctx context.Context, nickname string) (bool, error)
	Create(ctx context.Context, user *model.User, roleID uint) error
	FindRolesByUserID(ctx context.Context, userID uint) ([]string, error)
	TouchLoginPresence(ctx context.Context, userID uint) error
	UpdatePassword(ctx context.Context, userID uint, hashedPassword string) error
}

type authenticationRepo struct {
	db *gorm.DB
}

// NewAuthenticationRepository 创建仅供认证链路使用的 context-aware 仓储。
func NewAuthenticationRepository(db *gorm.DB) AuthenticationRepository {
	return &authenticationRepo{db: db}
}

func (r *authenticationRepo) FindByIdentifier(ctx context.Context, identifier string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Where("username = ? OR email = ? OR phone = ?", identifier, identifier, identifier).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *authenticationRepo) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *authenticationRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *authenticationRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

func (r *authenticationRepo) ExistsByNickname(ctx context.Context, nickname string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("nickname = ?", nickname).Count(&count).Error
	return count > 0, err
}

func (r *authenticationRepo) Create(ctx context.Context, user *model.User, roleID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return tx.Create(&model.UserRole{UserID: user.ID, RoleID: roleID}).Error
	})
}

func (r *authenticationRepo) FindRolesByUserID(ctx context.Context, userID uint) ([]string, error) {
	var names []string
	err := r.db.WithContext(ctx).Model(&model.UserRole{}).
		Joins("JOIN role ON role.id = user_role.role_id").
		Where("user_role.user_id = ?", userID).
		Pluck("role.name", &names).Error
	return names, err
}

func (r *authenticationRepo) TouchLoginPresence(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{
			"last_login_at":  gorm.Expr("NOW()"),
			"last_active_at": gorm.Expr("NOW()"),
		}).Error
}

func (r *authenticationRepo) UpdatePassword(ctx context.Context, userID uint, hashedPassword string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{"password": hashedPassword, "password_set": true}).Error
}
