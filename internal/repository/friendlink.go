package repository

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

const friendLinkVisibleStatus uint8 = 1

// FriendLinkUpdateData 友情链接更新数据；布尔字段表示对应属性是否参与更新。
type FriendLinkUpdateData struct {
	Name              *string
	Description       *string
	UpdateDescription bool
	Email             *string
	UpdateEmail       bool
	Phone             *string
	UpdatePhone       bool
	Site              *string
	AvatarUrl         *string
	UpdateAvatarUrl   bool
	Seq               *uint
	Status            *uint8
}

// FriendLinkRepository 友情链接数据访问接口。
type FriendLinkRepository interface {
	// ListPublic 查询显示中的友情链接，按 seq ASC、id DESC 排序。
	ListPublic(offset, limit int) ([]model.FriendLink, int64, error)
	// GetPublic 查询显示中的友情链接详情。
	GetPublic(id uint) (*model.FriendLink, error)
	// ListAdmin 查询管理端友情链接列表，可按状态过滤。
	ListAdmin(offset, limit int, status *uint8) ([]model.FriendLink, int64, error)
	// Create 创建友情链接。
	Create(link model.FriendLink) (*model.FriendLink, error)
	// Update 修改友情链接。
	Update(id uint, data FriendLinkUpdateData) (*model.FriendLink, error)
	// Delete 软删除友情链接。
	Delete(id uint) (*model.FriendLink, error)
}

type friendLinkRepo struct {
	db *gorm.DB
}

// NewFriendLinkRepository 创建友情链接仓储实例。
func NewFriendLinkRepository(db *gorm.DB) FriendLinkRepository {
	return &friendLinkRepo{db: db}
}

func (r *friendLinkRepo) ListPublic(offset, limit int) ([]model.FriendLink, int64, error) {
	// 公开列表只查询显示中的数据。
	query := r.db.Model(&model.FriendLink{}).Where("status = ?", friendLinkVisibleStatus)
	return listFriendLinks(query, offset, limit)
}

func (r *friendLinkRepo) GetPublic(id uint) (*model.FriendLink, error) {
	// 公开详情同样限制 status，避免隐藏数据被直接访问。
	var link model.FriendLink
	err := r.db.Where("status = ?", friendLinkVisibleStatus).First(&link, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &link, err
}

func (r *friendLinkRepo) ListAdmin(offset, limit int, status *uint8) ([]model.FriendLink, int64, error) {
	// 管理列表默认查看全部未删除数据，按需追加状态过滤。
	query := r.db.Model(&model.FriendLink{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	return listFriendLinks(query, offset, limit)
}

func (r *friendLinkRepo) Create(link model.FriendLink) (*model.FriendLink, error) {
	// 直接创建并返回 GORM 回填后的模型。
	if err := r.db.Create(&link).Error; err != nil {
		return nil, err
	}
	return &link, nil
}

func (r *friendLinkRepo) Update(id uint, data FriendLinkUpdateData) (*model.FriendLink, error) {
	// 先确认目标存在，再按显式字段更新，避免误创建或更新已删除数据。
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var link model.FriendLink
		if err := tx.First(&link, id).Error; err != nil {
			return err
		}

		fields := friendLinkUpdateFields(data)
		if len(fields) == 0 {
			return nil
		}
		return tx.Model(&model.FriendLink{}).Where("id = ?", id).Updates(fields).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return r.findByID(id)
}

func (r *friendLinkRepo) Delete(id uint) (*model.FriendLink, error) {
	// 软删除前先取出原记录，用于业务层返回删除对象。
	var link model.FriendLink
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&link, id).Error; err != nil {
			return err
		}
		return tx.Delete(&link).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &link, err
}

func (r *friendLinkRepo) findByID(id uint) (*model.FriendLink, error) {
	var link model.FriendLink
	err := r.db.First(&link, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &link, err
}

func listFriendLinks(query *gorm.DB, offset, limit int) ([]model.FriendLink, int64, error) {
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var links []model.FriendLink
	err := query.Order("seq ASC").Order("id DESC").Offset(offset).Limit(limit).Find(&links).Error
	return links, total, err
}

func friendLinkUpdateFields(data FriendLinkUpdateData) map[string]any {
	fields := make(map[string]any)
	if data.Name != nil {
		fields["name"] = *data.Name
	}
	if data.UpdateDescription {
		fields["description"] = data.Description
	}
	if data.UpdateEmail {
		fields["email"] = data.Email
	}
	if data.UpdatePhone {
		fields["phone"] = data.Phone
	}
	if data.Site != nil {
		fields["site"] = *data.Site
	}
	if data.UpdateAvatarUrl {
		fields["avatar_url"] = data.AvatarUrl
	}
	if data.Seq != nil {
		fields["seq"] = *data.Seq
	}
	if data.Status != nil {
		fields["status"] = *data.Status
	}
	return fields
}
