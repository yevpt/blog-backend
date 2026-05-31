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
	// 三字段 OR 查询，支持用户用任意一种标识符登录，前端无需区分类型
	err := r.db.Where("username = ? OR email = ? OR phone = ?", identifier, identifier, identifier).
		First(&user).Error
	// GORM 查不到记录时返回 ErrRecordNotFound，转换为 nil, nil 让调用方用 if user == nil 判断
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *userRepo) FindByID(id uint) (*model.User, error) {
	var user model.User
	// 按主键查询，First 找不到记录时返回 ErrRecordNotFound
	err := r.db.First(&user, id).Error
	// 统一转换为 nil, nil，调用方通过 if user == nil 判断，不需要解析错误类型
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *userRepo) ExistsByEmail(email string) (bool, error) {
	var count int64
	// Count 查询比 First 高效：只需走索引计数，无需回表读取完整行
	err := r.db.Model(&model.User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

func (r *userRepo) ExistsByNickname(nickname string) (bool, error) {
	var count int64
	// 同 ExistsByEmail，Count 查询避免不必要的全行读取
	err := r.db.Model(&model.User{}).Where("nickname = ?", nickname).Count(&count).Error
	return count > 0, err
}

func (r *userRepo) Create(user *model.User, roleID uint) error {
	// 事务保证用户表和角色关联表同时写入成功，避免出现有用户无角色的中间态
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 先写用户记录，自增主键写入后才能用于下一步的角色关联
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		// 写入用户-角色关联，绑定默认角色（NormalRole）
		return tx.Create(&model.UserRole{UserID: user.ID, RoleID: roleID}).Error
	})
}

func (r *userRepo) FindRolesByUserID(userID uint) ([]string, error) {
	var names []string
	// Join user_role 和 role 两张表，Pluck 只提取 role.name 字段，避免查询多余数据
	err := r.db.Model(&model.UserRole{}).
		Joins("JOIN role ON role.id = user_role.role_id").
		Where("user_role.user_id = ?", userID).
		Pluck("role.name", &names).Error
	return names, err
}

func (r *userRepo) UpdateLastLoginAt(userID uint) error {
	// 用 NOW() 由数据库生成时间，避免应用服务器与 DB 时区不一致带来的时间偏差
	return r.db.Model(&model.User{}).Where("id = ?", userID).
		Update("last_login_at", gorm.Expr("NOW()")).Error
}
