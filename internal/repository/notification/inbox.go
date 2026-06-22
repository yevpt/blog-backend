package notification

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm/clause"
)

// CreateInbox 幂等投递站内通知。
// 命中 uk_inbox_recipient_event 唯一约束时 DoNothing，RowsAffected 为 0，返回 created=false。
func (r *repo) CreateInbox(ctx context.Context, inbox *model.NotificationInbox) (bool, error) {
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(inbox)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ListInbox 分页查询某用户收件箱，并附带事件快照。
// 先取收件箱分页，再按事件 ID 批量取事件，避免 JOIN 扫描的脆弱性。
func (r *repo) ListInbox(ctx context.Context, recipientID uint, unreadOnly bool, page int, pageSize int) (*InboxPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// 统一的收件箱过滤条件：本人 + 可选未读。
	base := r.db.WithContext(ctx).Model(&model.NotificationInbox{}).
		Where("recipient_user_id = ?", recipientID)
	if unreadOnly {
		base = base.Where("is_read = ?", false)
	}

	// 先数总数，供前端分页。
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}
	result := &InboxPage{Total: total}
	if total == 0 {
		return result, nil
	}

	// 取当前页收件箱行，按时间倒序。
	var inboxes []model.NotificationInbox
	if err := base.
		Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&inboxes).Error; err != nil {
		return nil, err
	}

	// 批量取事件快照，组装聚合。
	events, err := r.eventsByIDs(ctx, collectEventIDs(inboxes))
	if err != nil {
		return nil, err
	}
	result.Items = make([]InboxAggregate, 0, len(inboxes))
	for _, inbox := range inboxes {
		result.Items = append(result.Items, InboxAggregate{
			Inbox: inbox,
			Event: events[inbox.EventID],
		})
	}
	return result, nil
}

// collectEventIDs 去重收集收件箱条目引用的事件 ID。
func collectEventIDs(inboxes []model.NotificationInbox) []uint {
	seen := make(map[uint]struct{}, len(inboxes))
	ids := make([]uint, 0, len(inboxes))
	for _, inbox := range inboxes {
		if _, ok := seen[inbox.EventID]; ok {
			continue
		}
		seen[inbox.EventID] = struct{}{}
		ids = append(ids, inbox.EventID)
	}
	return ids
}

// eventsByIDs 按 ID 批量取事件，返回以事件 ID 为键的映射。
func (r *repo) eventsByIDs(ctx context.Context, ids []uint) (map[uint]model.NotificationEvent, error) {
	result := make(map[uint]model.NotificationEvent, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var events []model.NotificationEvent
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&events).Error; err != nil {
		return nil, err
	}
	for _, event := range events {
		result[event.ID] = event
	}
	return result, nil
}

// CountUnread 统计某用户的未读数量。
func (r *repo) CountUnread(ctx context.Context, recipientID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.NotificationInbox{}).
		Where("recipient_user_id = ? AND is_read = ?", recipientID, false).
		Count(&count).Error
	return count, err
}

// MarkInboxRead 将某用户名下的单条通知置为已读。
// recipient_user_id 过滤即归属校验：非本人记录不会被更新，返回行数 0。
func (r *repo) MarkInboxRead(ctx context.Context, recipientID uint, id uint) (int64, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.NotificationInbox{}).
		Where("id = ? AND recipient_user_id = ? AND is_read = ?", id, recipientID, false).
		Updates(map[string]any{"is_read": true, "read_at": now})
	return res.RowsAffected, res.Error
}

// MarkAllInboxRead 批量已读；ids 为空表示该用户全部未读。
func (r *repo) MarkAllInboxRead(ctx context.Context, recipientID uint, ids []uint) (int64, error) {
	now := time.Now()
	q := r.db.WithContext(ctx).Model(&model.NotificationInbox{}).
		Where("recipient_user_id = ? AND is_read = ?", recipientID, false)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	res := q.Updates(map[string]any{"is_read": true, "read_at": now})
	return res.RowsAffected, res.Error
}

// DeleteInbox 软删除某用户名下的单条通知。
func (r *repo) DeleteInbox(ctx context.Context, recipientID uint, id uint) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("id = ? AND recipient_user_id = ?", id, recipientID).
		Delete(&model.NotificationInbox{})
	return res.RowsAffected, res.Error
}
