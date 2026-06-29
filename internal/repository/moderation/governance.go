package moderation

import (
	"context"
	"errors"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EnsureNewProfile 幂等创建新用户的默认审核画像。
func (r *repository) EnsureNewProfile(ctx context.Context, userID uint64, now time.Time) error {
	if userID == 0 || now.IsZero() {
		return ErrInvalidCommand
	}
	return r.db.WithContext(ctx).Exec(
		"INSERT INTO `user_moderation_profile` (`user_id`,`created_at`,`updated_at`) VALUES (?,?,?) ON DUPLICATE KEY UPDATE `user_id`=`user_id`",
		userID, now, now,
	).Error
}

// LoadModerationProfile 加锁读取画像，并懒释放已到期的自动限制和手工处罚。
func (r *repository) LoadModerationProfile(ctx context.Context, userID uint64, now time.Time) (ModerationProfile, error) {
	if userID == 0 || now.IsZero() {
		return ModerationProfile{}, ErrInvalidCommand
	}
	result := defaultModerationProfile(userID, now)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.UserModerationProfile
		err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		result = moderationProfileFromModel(row)
		updates := releaseExpiredProfile(&result, now)
		if len(updates) == 0 {
			return nil
		}
		updates["updated_at"] = now
		result.UpdatedAt = now
		return tx.WithContext(ctx).Model(&model.UserModerationProfile{}).
			Where("user_id = ?", userID).UpdateColumns(updates).Error
	})
	return result, err
}

// SetAutomaticTrust 仅在管理员未锁定画像时更新自动等级。
func (r *repository) SetAutomaticTrust(ctx context.Context, cmd AutomaticTrustCommand) (bool, error) {
	if cmd.UserID == 0 || !validTrustLevel(cmd.TrustLevel) || cmd.UpdatedAt.IsZero() {
		return false, ErrInvalidCommand
	}
	result := r.db.WithContext(ctx).Model(&model.UserModerationProfile{}).
		Where("user_id = ? AND manual_trust_locked = ?", cmd.UserID, false).
		UpdateColumns(map[string]any{
			"trust_level": cmd.TrustLevel, "trust_source": TrustSourceAuto,
			"restricted_until": cmd.RestrictedUntil, "updated_at": cmd.UpdatedAt,
		})
	return result.RowsAffected == 1, result.Error
}

// SetTrust 保存管理员校正结果；解除锁定后画像重新交由自动规则维护。
func (r *repository) SetTrust(ctx context.Context, cmd SetTrustCommand) error {
	if cmd.UserID == 0 || !validTrustLevel(cmd.TrustLevel) || cmd.UpdatedAt.IsZero() {
		return ErrInvalidCommand
	}
	source := TrustSourceAuto
	if cmd.ManualLocked {
		source = TrustSourceManual
	}
	result := r.db.WithContext(ctx).Model(&model.UserModerationProfile{}).Where("user_id = ?", cmd.UserID).
		UpdateColumns(map[string]any{
			"trust_level": cmd.TrustLevel, "trust_source": source,
			"manual_trust_locked": cmd.ManualLocked, "restricted_until": cmd.RestrictedUntil,
			"updated_at": cmd.UpdatedAt,
		})
	return requireProfileUpdate(result)
}

// SetSanction 设置禁言或封禁状态。
func (r *repository) SetSanction(ctx context.Context, cmd SetSanctionCommand) error {
	if cmd.UserID == 0 || (cmd.State != SanctionMuted && cmd.State != SanctionBanned) || cmd.Now.IsZero() {
		return ErrInvalidCommand
	}
	result := r.db.WithContext(ctx).Model(&model.UserModerationProfile{}).Where("user_id = ?", cmd.UserID).
		UpdateColumns(map[string]any{
			"sanction_state": cmd.State, "sanction_until": cmd.Until,
			"sanction_reason": cmd.Reason, "updated_at": cmd.Now,
		})
	return requireProfileUpdate(result)
}

// ReleaseSanction 由管理员立即解除处罚。
func (r *repository) ReleaseSanction(ctx context.Context, userID uint64, now time.Time) error {
	if userID == 0 || now.IsZero() {
		return ErrInvalidCommand
	}
	result := r.db.WithContext(ctx).Model(&model.UserModerationProfile{}).Where("user_id = ?", userID).
		UpdateColumns(map[string]any{
			"sanction_state": SanctionActive, "sanction_until": nil,
			"sanction_reason": nil, "updated_at": now,
		})
	return requireProfileUpdate(result)
}

func defaultModerationProfile(userID uint64, now time.Time) ModerationProfile {
	return ModerationProfile{
		UserID: userID, TrustLevel: TrustNew, TrustSource: TrustSourceAuto,
		SanctionState: SanctionActive, CreatedAt: now, UpdatedAt: now,
	}
}

func moderationProfileFromModel(row model.UserModerationProfile) ModerationProfile {
	return ModerationProfile{
		UserID: row.UserID, TrustLevel: TrustLevel(row.TrustLevel), TrustSource: TrustSource(row.TrustSource),
		ManualTrustLocked: row.ManualTrustLocked, SanctionState: SanctionState(row.SanctionState),
		SanctionUntil: row.SanctionUntil, SanctionReason: row.SanctionReason,
		CleanApprovalStreak: row.CleanApprovalStreak, CorrectedCount: row.CorrectedCount,
		RejectedCount: row.RejectedCount, HighRiskCount: row.HighRiskCount,
		ViolationScore: row.ViolationScore, LastViolationAt: row.LastViolationAt,
		RestrictedUntil: row.RestrictedUntil, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func releaseExpiredProfile(profile *ModerationProfile, now time.Time) map[string]any {
	updates := make(map[string]any)
	if profile.SanctionState != SanctionActive && expired(profile.SanctionUntil, now) {
		profile.SanctionState, profile.SanctionUntil, profile.SanctionReason = SanctionActive, nil, nil
		updates["sanction_state"], updates["sanction_until"], updates["sanction_reason"] = SanctionActive, nil, nil
	}
	if profile.TrustLevel == TrustRestricted && !profile.ManualTrustLocked && expired(profile.RestrictedUntil, now) {
		profile.TrustLevel, profile.RestrictedUntil, profile.ViolationScore = TrustNormal, nil, 0
		updates["trust_level"], updates["restricted_until"], updates["violation_score"] = TrustNormal, nil, 0
	}
	return updates
}

func expired(until *time.Time, now time.Time) bool {
	return until != nil && !until.After(now)
}

func validTrustLevel(level TrustLevel) bool {
	return level == TrustNew || level == TrustNormal || level == TrustTrusted || level == TrustRestricted
}

func requireProfileUpdate(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrSubjectNotFound
	}
	return nil
}
