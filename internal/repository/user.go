package repository

import (
	"errors"

	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/roles"
)

// UserDetailAggregate 用户详情聚合，供 service 层转换为 DTO。
type UserDetailAggregate struct {
	User        model.User
	Roles       []string
	Meta        *model.UserMeta
	Setting     *model.UserSetting
	SocialLinks []model.UserSocialLink
}

// UserRepository 用户数据访问接口，所有方法返回 model 而非 dto，转换由上层负责
type UserRepository interface {
	// FindByIdentifier 支持 username / email / phone 三合一查询；未找到时返回 nil, nil
	FindByIdentifier(identifier string) (*model.User, error)
	// FindByID 按主键查询；未找到时返回 nil, nil
	FindByID(id uint) (*model.User, error)
	// FindDetailByID 查询用户详情聚合，包含角色、扩展资料、偏好设置和社交链接。
	FindDetailByID(id uint) (*UserDetailAggregate, error)
	ExistsByEmail(email string) (bool, error)
	ExistsByNickname(nickname string) (bool, error)
	// Create 在事务中同时插入用户记录和角色关联，保证数据一致性
	Create(user *model.User, roleID uint) error
	// FindRolesByUserID 返回用户所有角色名称列表，供 JWT 签发时填充 claims
	FindRolesByUserID(userID uint) ([]string, error)
	// FindRolesByUserIDs 批量查询用户角色列表，返回以 user_id 为 key 的字典
	FindRolesByUserIDs(userIDs []uint) (map[uint][]string, error)
	UpdateLastLoginAt(userID uint) error
	// ListRecent 获取最近访问的用户列表，按最后登录时间降序
	ListRecent(offset, limit int) ([]model.User, int64, error)
	// ListAll 获取所有用户列表，按角色排序 (admin > vip > normal)，然后按最后登录时间降序
	ListAll(offset, limit int) ([]model.User, int64, error)
	// Update 更新用户信息
	Update(id uint, updates map[string]interface{}) error
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

func (r *userRepo) FindDetailByID(id uint) (*UserDetailAggregate, error) {
	// 先读取主用户记录，不存在时直接返回 nil。
	user, err := r.FindByID(id)
	if err != nil || user == nil {
		return nil, err
	}

	// 再补齐角色、扩展信息、偏好设置和社交链接。
	roles, err := r.FindRolesByUserID(id)
	if err != nil {
		return nil, err
	}
	meta, err := r.findUserMetaByUserID(id)
	if err != nil {
		return nil, err
	}
	setting, err := r.findUserSettingByUserID(id)
	if err != nil {
		return nil, err
	}
	socialLinks, err := r.findUserSocialLinksByUserID(id)
	if err != nil {
		return nil, err
	}

	// 返回 repository 层聚合，DTO 转换交给 service。
	return &UserDetailAggregate{
		User:        *user,
		Roles:       roles,
		Meta:        meta,
		Setting:     setting,
		SocialLinks: socialLinks,
	}, nil
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

func (r *userRepo) FindRolesByUserIDs(userIDs []uint) (map[uint][]string, error) {
	if len(userIDs) == 0 {
		return make(map[uint][]string), nil
	}

	type userRoleResult struct {
		UserID   uint   `gorm:"column:user_id"`
		RoleName string `gorm:"column:name"`
	}
	var results []userRoleResult

	err := r.db.Model(&model.UserRole{}).
		Select("user_role.user_id, role.name").
		Joins("JOIN role ON role.id = user_role.role_id").
		Where("user_role.user_id IN ?", userIDs).
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	rolesMap := make(map[uint][]string)
	for _, res := range results {
		rolesMap[res.UserID] = append(rolesMap[res.UserID], res.RoleName)
	}
	return rolesMap, nil
}

func (r *userRepo) UpdateLastLoginAt(userID uint) error {
	// 用 NOW() 由数据库生成时间，避免应用服务器与 DB 时区不一致带来的时间偏差
	return r.db.Model(&model.User{}).Where("id = ?", userID).
		Update("last_login_at", gorm.Expr("NOW()")).Error
}

func (r *userRepo) ListRecent(offset, limit int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	// 只查询 status = 1 的用户
	query := r.db.Model(&model.User{}).Where("status = ?", 1)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("COALESCE(last_login_at, created_at) DESC, id DESC").Offset(offset).Limit(limit).Find(&users).Error
	return users, total, err
}

func (r *userRepo) ListAll(offset, limit int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	// 只查询 status = 1 的用户
	query := r.db.Model(&model.User{}).Where("status = ?", 1)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 按角色名称映射业务权重排序，避免数据库自增 id 顺序和权限权重不一致。
	// 一个用户可能有多个角色，取最小权重代表该用户最高权限。
	roleWeightExpr := listUserRoleWeightExpr()

	err := r.db.Table("user").
		Select("DISTINCT user.*").
		Joins("LEFT JOIN user_role ON user_role.user_id = user.id").
		Joins("LEFT JOIN role ON role.id = user_role.role_id").
		Where("user.status = ?", 1).
		Group("user.id").
		Order(roleWeightExpr + " ASC, COALESCE(user.last_login_at, user.created_at) DESC, user.id DESC").
		Offset(offset).
		Limit(limit).
		Find(&users).Error

	return users, total, err
}

func listUserRoleWeightExpr() string {
	return "MIN(CASE role.name WHEN '" + roles.AdminRole + "' THEN 1 WHEN '" + roles.VipRole + "' THEN 2 WHEN '" + roles.NormalRole + "' THEN 3 ELSE 999 END)"
}

func (r *userRepo) Update(id uint, updates map[string]interface{}) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

func (r *userRepo) findUserMetaByUserID(userID uint) (*model.UserMeta, error) {
	var meta model.UserMeta
	// 用户扩展资料是 1:1 关系，缺失时按 nil 处理。
	err := r.db.Where("user_id = ?", userID).First(&meta).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &meta, err
}

func (r *userRepo) findUserSettingByUserID(userID uint) (*model.UserSetting, error) {
	var setting model.UserSetting
	// 用户偏好设置是 1:1 关系，缺失时返回 nil 让上层按未配置处理。
	err := r.db.Where("user_id = ?", userID).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &setting, err
}

func (r *userRepo) findUserSocialLinksByUserID(userID uint) ([]model.UserSocialLink, error) {
	var links []model.UserSocialLink
	// 社交链接按平台名稳定排序，便于前端渲染与测试断言。
	err := r.db.Where("user_id = ?", userID).Order("platform ASC").Find(&links).Error
	return links, err
}
