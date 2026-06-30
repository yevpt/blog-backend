package moderation

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ApplyPublishedImageKeys 在单事务内锁定审核项、校验版本仍为当前物化或通过版本，
// 批量更新修订图片 key 为正式 key，将被删除旧公开图片引用改为审计 key，并重建 moment_media。
func (r *repository) ApplyPublishedImageKeys(ctx context.Context, cmd PublishedImageCommand) error {
	if cmd.ItemID == 0 || cmd.RevisionID == 0 || cmd.MomentID == 0 || cmd.AuthorID == 0 {
		return ErrInvalidCommand
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁定审核项，确认版本归属未被并发改变。
		var item model.ModerationItem
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", cmd.ItemID).Take(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrItemNotFound
			}
			return err
		}
		if !revisionIsCurrent(item, cmd.RevisionID) {
			return ErrRevisionStateConflict
		}
		// 更新当前版本图片引用为正式 key。
		now := tx.NowFunc()
		for _, key := range cmd.ImageKeys {
			result := tx.WithContext(ctx).Model(&model.ModerationRevisionImage{}).
				Where("revision_id = ? AND seq = ?", cmd.RevisionID, key.Seq).
				Updates(map[string]any{"object_key": key.ObjectKey, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
		}
		// 将旧公开图片引用改为审计 key。
		for _, move := range cmd.AuditMoves {
			result := tx.WithContext(ctx).Model(&model.ModerationRevisionImage{}).
				Where("object_key = ?", move.OldObjectKey).
				Updates(map[string]any{"object_key": move.NewObjectKey, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
		}
		return rebuildMomentMedia(ctx, tx, cmd.MomentID, cmd.AuthorID, cmd.RevisionID)
	})
}

// revisionIsCurrent 判断版本是否仍为审核项的物化或通过版本。
func revisionIsCurrent(item model.ModerationItem, revisionID uint64) bool {
	if item.MaterializedRevisionID != nil && *item.MaterializedRevisionID == revisionID {
		return true
	}
	if item.ApprovedRevisionID != nil && *item.ApprovedRevisionID == revisionID {
		return true
	}
	return false
}

// rebuildMomentMedia 删除并按当前版本图片重建碎语媒体表。
func rebuildMomentMedia(ctx context.Context, tx *gorm.DB, momentID, authorID, revisionID uint64) error {
	if err := tx.WithContext(ctx).Unscoped().Where("moment_id = ?", momentID).Delete(&model.Media{}).Error; err != nil {
		return err
	}
	var images []model.ModerationRevisionImage
	if err := tx.WithContext(ctx).Where("revision_id = ?", revisionID).Order("seq ASC,id ASC").Find(&images).Error; err != nil {
		return err
	}
	if len(images) == 0 {
		return nil
	}
	now := tx.NowFunc()
	rows := make([]model.Media, 0, len(images))
	for _, image := range images {
		name := path.Base(image.ObjectKey)
		rows = append(rows, model.Media{
			UploaderID: uint(authorID), MomentID: uint(momentID), Type: uint8(0),
			FileType: strings.TrimPrefix(strings.ToLower(path.Ext(name)), "."), Name: name,
			URL: image.ObjectKey, Size: uint(image.Size), Status: 1, Seq: image.Seq,
			Base: model.Base{CreatedAt: now, UpdatedAt: now},
		})
	}
	return tx.WithContext(ctx).Create(&rows).Error
}
