package user

import (
	"errors"
	"strings"
	"time"

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

const (
	// LikedContentFilterArticle 表示文章筛选。
	LikedContentFilterArticle = "article"
	// LikedContentFilterComment 表示评论筛选，包含评论与回复。
	LikedContentFilterComment = "comment"
	// LikedContentFilterGuestbook 表示留言筛选。
	LikedContentFilterGuestbook = "guestbook"
	// LikedContentFilterMoment 表示碎语筛选。
	LikedContentFilterMoment = "moment"

	// LikedContentKindArticle 表示点赞目标为文章。
	LikedContentKindArticle = "article"
	// LikedContentKindComment 表示点赞目标为一级评论。
	LikedContentKindComment = "comment"
	// LikedContentKindReply 表示点赞目标为回复。
	LikedContentKindReply = "reply"
	// LikedContentKindGuestbook 表示点赞目标为留言。
	LikedContentKindGuestbook = "guestbook"
	// LikedContentKindMoment 表示点赞目标为碎语。
	LikedContentKindMoment = "moment"

	// LikedContentRootArticle 表示根对象为文章。
	LikedContentRootArticle = "article"
	// LikedContentRootMoment 表示根对象为碎语。
	LikedContentRootMoment = "moment"
	// LikedContentRootGuestbook 表示根对象为留言板。
	LikedContentRootGuestbook = "guestbook"
)

// LikedContentFilter 用户点赞内容查询过滤条件。
type LikedContentFilter struct {
	UserID   uint
	Type     string
	Page     int
	PageSize int
}

// LikedContentObject 点赞内容、父级或根对象摘要，供 service 转 DTO。
type LikedContentObject struct {
	ID          uint
	Kind        string
	Title       *string
	Excerpt     string
	CoverImgURL *string
	Deleted     bool
}

// LikedContentStats 点赞内容的轻量统计。
type LikedContentStats struct {
	LikeCount    *int64
	CommentCount *int64
}

// LikedContentAggregate 用户点赞内容聚合。
type LikedContentAggregate struct {
	ID      uint
	LikedAt time.Time
	Kind    string
	Filter  string
	Author  *model.User
	Content LikedContentObject
	Parent  *LikedContentObject
	Root    *LikedContentObject
	ToUser  *model.User
	Stats   *LikedContentStats
}

// LikedContentPageResult 用户点赞内容分页查询结果。
type LikedContentPageResult struct {
	Total    int64
	Page     int
	PageSize int
	Items    []LikedContentAggregate
}

// UserRepository 用户数据访问接口，所有方法返回 model 而非 dto，转换由上层负责
type UserRepository interface {
	// FindByIdentifier 支持 username / email / phone 三合一查询；未找到时返回 nil, nil
	FindByIdentifier(identifier string) (*model.User, error)
	// FindByUsername 仅按 username 查询；未找到时返回 nil, nil
	FindByUsername(username string) (*model.User, error)
	// FindByEmail 仅按主邮箱查询；未找到时返回 nil, nil。
	FindByEmail(email string) (*model.User, error)
	// FindByID 按主键查询；未找到时返回 nil, nil
	FindByID(id uint) (*model.User, error)
	// FindDetailByID 查询用户详情聚合，包含角色、扩展资料、偏好设置和社交链接。
	FindDetailByID(id uint) (*UserDetailAggregate, error)
	// ListLikedContent 分页查询某个用户赞过的公开内容。
	ListLikedContent(filter LikedContentFilter) (*LikedContentPageResult, error)
	// CountLikedContent 统计某个用户赞过的公开内容总数。
	CountLikedContent(userID uint) (int64, error)
	ExistsByEmail(email string) (bool, error)
	// EmailInUseByOther 检查主邮箱或副邮箱是否已被其他用户占用。
	EmailInUseByOther(email string, excludeID uint) (bool, error)
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
	Update(id uint, updates map[string]any) error
	// ExistsByUsername 检查用户名是否已被占用（排除自身）
	ExistsByUsername(username string, excludeID uint) (bool, error)
	// UpdatePassword 直接写入已哈希的密码
	UpdatePassword(userID uint, hashedPassword string) error
	// UpsertMeta 创建或更新用户扩展资料（按 userID 主键 upsert）
	UpsertMeta(userID uint, updates map[string]any) error
	// UpsertSocialLink 创建或更新指定平台的社交链接
	UpsertSocialLink(userID uint, platform, url string) error
	// DeleteSocialLink 删除指定平台的社交链接
	DeleteSocialLink(userID uint, platform string) error
	// UpsertUserSetting 创建或更新用户偏好设置
	UpsertUserSetting(userID uint, updates map[string]any) error
	// CountByAvatarURL 统计使用指定头像 key 的用户数量。
	CountByAvatarURL(avatarURL string) (int64, error)
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

func (r *userRepo) FindByUsername(username string) (*model.User, error) {
	var user model.User
	// 管理后台入口只允许用户名登录，避免邮箱或手机号绕过入口语义。
	err := r.db.Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *userRepo) FindByEmail(email string) (*model.User, error) {
	var user model.User
	err := r.db.Where("email = ?", email).First(&user).Error
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

func (r *userRepo) EmailInUseByOther(email string, excludeID uint) (bool, error) {
	var mainCount int64
	if err := r.db.Model(&model.User{}).
		Where("email = ? AND id != ?", email, excludeID).
		Count(&mainCount).Error; err != nil {
		return false, err
	}
	if mainCount > 0 {
		return true, nil
	}

	var subCount int64
	err := r.db.Model(&model.UserMeta{}).
		Where("sub_email = ? AND user_id != ?", email, excludeID).
		Count(&subCount).Error
	return subCount > 0, err
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

func (r *userRepo) Update(id uint, updates map[string]any) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

func (r *userRepo) ExistsByUsername(username string, excludeID uint) (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).
		Where("username = ? AND id != ?", username, excludeID).
		Count(&count).Error
	return count > 0, err
}

func (r *userRepo) UpdatePassword(userID uint, hashedPassword string) error {
	return r.db.Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{"password": hashedPassword, "password_set": true}).Error
}

func (r *userRepo) UpsertMeta(userID uint, updates map[string]any) error {
	// 先查是否存在，存在则 Update，否则 Create（含 userID 主键）
	var meta model.UserMeta
	err := r.db.Where("user_id = ?", userID).First(&meta).Error
	if err != nil {
		// 不存在，插入
		updates["user_id"] = userID
		return r.db.Model(&model.UserMeta{}).Create(updates).Error
	}
	return r.db.Model(&model.UserMeta{}).Where("user_id = ?", userID).Updates(updates).Error
}

func (r *userRepo) UpsertSocialLink(userID uint, platform, url string) error {
	// 用 ON DUPLICATE KEY UPDATE 语义：先尝试插入，已存在则更新
	link := model.UserSocialLink{UserID: userID, Platform: platform, URL: url}
	return r.db.Where(model.UserSocialLink{UserID: userID, Platform: platform}).
		Assign(model.UserSocialLink{URL: url}).
		FirstOrCreate(&link).Error
}

func (r *userRepo) DeleteSocialLink(userID uint, platform string) error {
	return r.db.Where("user_id = ? AND platform = ?", userID, platform).
		Delete(&model.UserSocialLink{}).Error
}

func (r *userRepo) UpsertUserSetting(userID uint, updates map[string]any) error {
	var setting model.UserSetting
	err := r.db.Where("user_id = ?", userID).First(&setting).Error
	if err != nil {
		updates["user_id"] = userID
		return r.db.Model(&model.UserSetting{}).Create(updates).Error
	}
	return r.db.Model(&model.UserSetting{}).Where("user_id = ?", userID).Updates(updates).Error
}

func (r *userRepo) CountByAvatarURL(avatarURL string) (int64, error) {
	avatarURL = strings.TrimSpace(avatarURL)
	if avatarURL == "" {
		return 0, nil
	}
	var count int64
	err := r.db.Model(&model.User{}).Where("avatar_url = ?", avatarURL).Count(&count).Error
	return count, err
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
