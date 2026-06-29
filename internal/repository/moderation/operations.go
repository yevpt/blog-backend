package moderation

import (
	"context"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LoadControl 读取全站审核控制单例。
func (r *repository) LoadControl(ctx context.Context) (ControlRecord, error) {
	var row model.ModerationControl
	if err := r.db.WithContext(ctx).Where("id = ?", uint64(1)).Take(&row).Error; err != nil {
		return ControlRecord{}, err
	}
	return controlRecord(row), nil
}

// UpdateControl 使用 lock_version 防止管理员页面旧数据覆盖新控制状态。
func (r *repository) UpdateControl(ctx context.Context, cmd UpdateControlCommand) error {
	if !validRegistrationMode(cmd.RegistrationMode) || !validPublishingMode(cmd.PublishingMode) ||
		cmd.OperatorID == 0 || cmd.ExpectedLockVersion == 0 || cmd.ChangedAt.IsZero() {
		return ErrInvalidCommand
	}
	result := r.db.WithContext(ctx).Model(&model.ModerationControl{}).
		Where("id = ? AND lock_version = ?", uint64(1), cmd.ExpectedLockVersion).
		UpdateColumns(map[string]any{
			"registration_mode": cmd.RegistrationMode, "publishing_mode": cmd.PublishingMode,
			"reason": cmd.Reason, "operator_id": cmd.OperatorID, "changed_at": cmd.ChangedAt,
			"lock_version": gorm.Expr("lock_version + 1"), "updated_at": cmd.ChangedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrOptimisticLock
	}
	return nil
}

// ApplyUserEmergencyBatch 在单个事务内分批隐藏或恢复用户公开内容。
func (r *repository) ApplyUserEmergencyBatch(ctx context.Context, cmd UserEmergencyBatchCommand) (EmergencyBatchResult, error) {
	if cmd.UserID == 0 || cmd.ActorID == 0 || cmd.Limit <= 0 || cmd.Now.IsZero() || (cmd.Hide && cmd.Reason == nil) {
		return EmergencyBatchResult{}, ErrInvalidCommand
	}
	var result EmergencyBatchResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids, err := lockEmergencyCandidates(ctx, tx, cmd)
		if err != nil || len(ids) == 0 {
			return err
		}
		if len(ids) > cmd.Limit {
			result.HasMore = true
			ids = ids[:cmd.Limit]
		}
		if err := updateEmergencyCandidates(ctx, tx, cmd, ids); err != nil {
			return err
		}
		if err := appendEmergencyLogs(ctx, tx, cmd, ids); err != nil {
			return err
		}
		result.Processed = len(ids)
		result.NextCursor = ids[len(ids)-1]
		return nil
	})
	return result, err
}

func lockEmergencyCandidates(ctx context.Context, tx *gorm.DB, cmd UserEmergencyBatchCommand) ([]uint64, error) {
	query := tx.WithContext(ctx).Table("moderation_item").Select("id").
		Where("author_id = ? AND id > ? AND lifecycle_state = ?", cmd.UserID, cmd.Cursor, LifecycleActive)
	if cmd.Hide {
		query = query.Where("public_state = ? AND pending_revision_id IS NULL AND approved_revision_id IS NOT NULL AND materialized_revision_id = approved_revision_id", PublicVisible)
	} else {
		query = query.Where("public_state = ? AND state_before_emergency IS NOT NULL", PublicEmergencyHidden)
	}
	var ids []uint64
	err := query.Order("id ASC").Limit(cmd.Limit + 1).Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&ids).Error
	return ids, err
}

func updateEmergencyCandidates(ctx context.Context, tx *gorm.DB, cmd UserEmergencyBatchCommand, ids []uint64) error {
	updates := map[string]any{"lock_version": gorm.Expr("lock_version + 1"), "updated_at": cmd.Now}
	if cmd.Hide {
		updates["state_before_emergency"] = gorm.Expr("public_state")
		updates["public_state"] = PublicEmergencyHidden
		updates["emergency_hidden_reason"] = cmd.Reason
		updates["emergency_hidden_at"] = cmd.Now
	} else {
		updates["public_state"] = gorm.Expr("state_before_emergency")
		updates["state_before_emergency"] = nil
		updates["emergency_hidden_reason"] = nil
		updates["emergency_hidden_at"] = nil
	}
	result := tx.WithContext(ctx).Model(&model.ModerationItem{}).Where("id IN ?", ids).UpdateColumns(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != int64(len(ids)) {
		return ErrOptimisticLock
	}
	return nil
}

func appendEmergencyLogs(ctx context.Context, tx *gorm.DB, cmd UserEmergencyBatchCommand, ids []uint64) error {
	event := EventRestore
	if cmd.Hide {
		event = EventEmergencyHide
	}
	logs := make([]model.ModerationActionLog, 0, len(ids))
	for _, itemID := range ids {
		id, actorID, subjectID := itemID, cmd.ActorID, cmd.UserID
		logs = append(logs, model.ModerationActionLog{
			ItemID: &id, ActorUserID: &actorID, SubjectUserID: &subjectID,
			Action: string(event), Reason: cmd.Reason, CreatedAt: cmd.Now,
		})
	}
	return tx.WithContext(ctx).Create(&logs).Error
}

func controlRecord(row model.ModerationControl) ControlRecord {
	return ControlRecord{
		RegistrationMode: RegistrationMode(row.RegistrationMode), PublishingMode: PublishingMode(row.PublishingMode),
		Reason: row.Reason, OperatorID: row.OperatorID, ChangedAt: row.ChangedAt, LockVersion: row.LockVersion,
	}
}

func validRegistrationMode(mode RegistrationMode) bool {
	return mode == RegistrationOpen || mode == RegistrationClosed
}

func validPublishingMode(mode PublishingMode) bool {
	return mode == PublishingOpen || mode == PublishingPreReviewAll || mode == PublishingClosed
}
