package notification

import (
	"context"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetQuotaPolicies 读取全部 purpose 额度策略。
func (r *repo) GetQuotaPolicies(ctx context.Context) ([]model.EmailQuotaPolicy, error) {
	var policies []model.EmailQuotaPolicy
	err := r.db.WithContext(ctx).Find(&policies).Error
	return policies, err
}

// GetRoleQuotaPolicies 读取全部角色额度策略。
func (r *repo) GetRoleQuotaPolicies(ctx context.Context) ([]model.EmailRoleQuotaPolicy, error) {
	var policies []model.EmailRoleQuotaPolicy
	err := r.db.WithContext(ctx).Find(&policies).Error
	return policies, err
}

// ReserveQuota 原子占用一次额度。
//
// 两步同事务：先以唯一键 DoNothing 保证用量行存在（不覆盖既有计数），
// 再用「used_count < limit」的条件自增。条件自增的 RowsAffected==1 表示占用成功，
// 为 0 表示已达上限。把上限判断交给数据库的条件更新，避免多 worker 读改写竞争超发。
func (r *repo) ReserveQuota(ctx context.Context, key QuotaUsageKey, limit int) (bool, error) {
	allowed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 确保用量行存在；命中唯一约束时不动既有计数。
		usage := model.EmailQuotaUsage{
			QuotaDate:   key.QuotaDate,
			ScopeType:   key.ScopeType,
			ScopeID:     key.ScopeID,
			Purpose:     key.Purpose,
			WindowType:  key.WindowType,
			WindowStart: key.WindowStart,
			UsedCount:   0,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&usage).Error; err != nil {
			return err
		}

		// 条件自增：仅在未达上限时 +1。
		res := tx.Model(&model.EmailQuotaUsage{}).
			Where("scope_type = ? AND scope_id = ? AND purpose = ? AND window_type = ? AND window_start = ?",
				key.ScopeType, key.ScopeID, key.Purpose, key.WindowType, key.WindowStart).
			Where("used_count < ?", limit).
			UpdateColumn("used_count", gorm.Expr("used_count + 1"))
		if res.Error != nil {
			return res.Error
		}
		allowed = res.RowsAffected == 1
		return nil
	})
	return allowed, err
}
