package repository

import (
	"errors"

	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
)

// UserRepository 用户数据访问接口，所有方法返回 model 而非 dto，转换由上层负责
type UserRepository interface {
	// FindByIdentifier 支持 username / email / phone 三合一查询；未找到时返回 nil, nil
	FindByIdentifier(identifier string) (*model.User, error)
	// FindByID 按主键查询；未找到时返回 nil, nil
	FindByID(id uint) (*model.User, error)
	ExistsByEmail(email string) (bool, error)
	ExistsByNickname(nickname string) (bool, error)
	// Create 在事务中同时插入用户记录和角色关联，保证数据一致性
	Create(user *model.User, roleID uint) error
	// FindRolesByUserID 返回用户所有角色名称列表，供 JWT 签发时填充 claims
	FindRolesByUserID(userID uint) ([]string, error)
	UpdateLastLoginAt(userID uint) error
}

type userRepo struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepo{db: db}
}

func (r *userRepo) FindByIdentifier(identifier string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ? OR email = ? OR phone = ?", identifier, identifier, identifier).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *userRepo) FindByID(id uint) (*model.User, error) {
	var user model.User
	err := r.db.First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *userRepo) ExistsByEmail(email string) (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

func (r *userRepo) ExistsByNickname(nickname string) (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).Where("nickname = ?", nickname).Count(&count).Error
	return count > 0, err
}

func (r *userRepo) Create(user *model.User, roleID uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return tx.Create(&model.UserRole{UserID: user.ID, RoleID: roleID}).Error
	})
}

func (r *userRepo) FindRolesByUserID(userID uint) ([]string, error) {
	var names []string
	err := r.db.Model(&model.UserRole{}).
		Joins("JOIN role ON role.id = user_role.role_id").
		Where("user_role.user_id = ?", userID).
		Pluck("role.name", &names).Error
	return names, err
}

func (r *userRepo) UpdateLastLoginAt(userID uint) error {
	return r.db.Model(&model.User{}).Where("id = ?", userID).
		Update("last_login_at", gorm.Expr("NOW()")).Error
}
