package notification

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// AdminRepository 管理端通知查询与额度调整。
// 独立于 Repository 聚合，避免影响业务侧 mock；由同一 *repo 实现。
type AdminRepository interface {
	// ListEmailTasks 按状态分页查询邮件任务，status 为空表示不过滤。
	ListEmailTasks(ctx context.Context, status string, page, pageSize int) ([]model.NotificationEmailTask, int64, error)
	// ListEmailBatches 按状态分页查询邮件批次，status 为空表示不过滤。
	ListEmailBatches(ctx context.Context, status string, page, pageSize int) ([]model.NotificationEmailBatch, int64, error)
	// ListQuotaPolicies 读取全部 purpose 额度策略。
	ListQuotaPolicies(ctx context.Context) ([]model.EmailQuotaPolicy, error)
	// ListRoleQuotaPolicies 读取全部角色额度策略。
	ListRoleQuotaPolicies(ctx context.Context) ([]model.EmailRoleQuotaPolicy, error)
	// UpdateQuotaPolicy 更新 purpose 额度策略字段，返回受影响行数。
	UpdateQuotaPolicy(ctx context.Context, id uint, fields map[string]any) (int64, error)
	// UpdateRoleQuotaPolicy 更新角色额度策略字段，返回受影响行数。
	UpdateRoleQuotaPolicy(ctx context.Context, id uint, fields map[string]any) (int64, error)
	// RetryBatch 把失败/延后的批次重置为 pending 并立即到点，返回受影响行数。
	RetryBatch(ctx context.Context, id uint) (int64, error)
}

// NewAdminRepository 创建管理端通知仓储（与业务仓储共用底层 *repo）。
func NewAdminRepository(db *gorm.DB) AdminRepository {
	return &repo{db: db}
}

// ListEmailTasks 按状态分页查询邮件任务。
func (r *repo) ListEmailTasks(ctx context.Context, status string, page, pageSize int) ([]model.NotificationEmailTask, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.NotificationEmailTask{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tasks []model.NotificationEmailTask
	if err := query.Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// ListEmailBatches 按状态分页查询邮件批次。
func (r *repo) ListEmailBatches(ctx context.Context, status string, page, pageSize int) ([]model.NotificationEmailBatch, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.NotificationEmailBatch{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var batches []model.NotificationEmailBatch
	if err := query.Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&batches).Error; err != nil {
		return nil, 0, err
	}
	return batches, total, nil
}

// ListQuotaPolicies 读取全部 purpose 额度策略。
func (r *repo) ListQuotaPolicies(ctx context.Context) ([]model.EmailQuotaPolicy, error) {
	return r.GetQuotaPolicies(ctx)
}

// ListRoleQuotaPolicies 读取全部角色额度策略。
func (r *repo) ListRoleQuotaPolicies(ctx context.Context) ([]model.EmailRoleQuotaPolicy, error) {
	return r.GetRoleQuotaPolicies(ctx)
}

// UpdateQuotaPolicy 更新 purpose 额度策略字段。
func (r *repo) UpdateQuotaPolicy(ctx context.Context, id uint, fields map[string]any) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.EmailQuotaPolicy{}).
		Where("id = ?", id).Updates(fields)
	return res.RowsAffected, res.Error
}

// UpdateRoleQuotaPolicy 更新角色额度策略字段。
func (r *repo) UpdateRoleQuotaPolicy(ctx context.Context, id uint, fields map[string]any) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.EmailRoleQuotaPolicy{}).
		Where("id = ?", id).Updates(fields)
	return res.RowsAffected, res.Error
}

// RetryBatch 把失败/延后的批次重置为 pending 并立即到点，释放租约清空错误。
func (r *repo) RetryBatch(ctx context.Context, id uint) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.NotificationEmailBatch{}).
		Where("id = ? AND status IN ?", id, []string{EmailBatchStatusFailed, EmailBatchStatusDeferred}).
		Updates(map[string]any{
			"status":       EmailBatchStatusPending,
			"scheduled_at": time.Now(),
			"lease_until":  nil,
			"locked_by":    nil,
			"last_error":   nil,
		})
	return res.RowsAffected, res.Error
}
