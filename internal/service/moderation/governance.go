package moderation

import (
	"context"
	"strings"
	"time"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/pkg/config"
)

const (
	TrustSourceAuto   = "auto"
	TrustSourceManual = "manual"
)

// UserModerationProfile 是 service 层使用的用户审核画像，不直接暴露数据库模型。
type UserModerationProfile struct {
	UserID              uint64
	TrustLevel          TrustLevel
	TrustSource         string
	ManualTrustLocked   bool
	SanctionState       SanctionState
	SanctionUntil       *time.Time
	SanctionReason      *string
	CleanApprovalStreak uint64
	CorrectedCount      uint64
	RejectedCount       uint64
	HighRiskCount       uint64
	ViolationScore      int64
	LastViolationAt     *time.Time
	RestrictedUntil     *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// SetTrustCommand 是管理员校正用户信任等级的命令。
type SetTrustCommand struct {
	UserID          uint64
	ActorID         uint64
	TrustLevel      TrustLevel
	ManualLocked    bool
	RestrictedUntil *time.Time
}

// SetSanctionCommand 是管理员设置禁言或封禁的命令。
type SetSanctionCommand struct {
	UserID  uint64
	ActorID uint64
	State   SanctionState
	Until   *time.Time
	Reason  string
}

// GovernanceService 管理轻量信任等级和管理员处罚。
type GovernanceService interface {
	EnsureNewProfile(ctx context.Context, userID uint64) error
	RegistrationAllowed(ctx context.Context) (bool, error)
	PublishingAllowed(ctx context.Context, userID uint64) (bool, error)
	GetProfile(ctx context.Context, userID uint64) (UserModerationProfile, error)
	SetTrust(ctx context.Context, cmd SetTrustCommand) error
	SetSanction(ctx context.Context, cmd SetSanctionCommand) error
	ReleaseSanction(ctx context.Context, userID, actorID uint64) error
}

type governanceService struct {
	repo moderationrepo.Repository
	cfg  config.ModerationGovernanceConfig
	now  func() time.Time
}

// NewGovernanceService 通过构造注入创建用户审核治理服务。
func NewGovernanceService(repo moderationrepo.Repository, cfg config.ModerationGovernanceConfig, now func() time.Time) GovernanceService {
	if now == nil {
		now = time.Now
	}
	return &governanceService{repo: repo, cfg: cfg, now: now}
}

// EnsureNewProfile 幂等创建新注册用户的默认画像。
func (s *governanceService) EnsureNewProfile(ctx context.Context, userID uint64) error {
	if userID == 0 {
		return ErrInvalidRequest
	}
	return s.repo.EnsureNewProfile(ctx, userID, s.now())
}

// RegistrationAllowed 返回当前全站注册开关；数据库是唯一事实源。
func (s *governanceService) RegistrationAllowed(ctx context.Context) (bool, error) {
	control, err := s.repo.LoadControl(ctx)
	if err != nil {
		return false, err
	}
	return control.RegistrationMode == moderationrepo.RegistrationOpen, nil
}

// PublishingAllowed 判断用户处罚和全站开关是否允许创建待发布资源。
func (s *governanceService) PublishingAllowed(ctx context.Context, userID uint64) (bool, error) {
	if userID == 0 {
		return false, ErrInvalidRequest
	}
	policy, err := s.repo.LoadPolicyContext(ctx, userID)
	if err != nil {
		return false, err
	}
	return policy.SanctionState == moderationrepo.SanctionActive && policy.PublishingMode != moderationrepo.PublishingClosed, nil
}

// GetProfile 读取画像、释放到期限制，并持久化本次自动等级计算。
func (s *governanceService) GetProfile(ctx context.Context, userID uint64) (UserModerationProfile, error) {
	if userID == 0 {
		return UserModerationProfile{}, ErrInvalidRequest
	}
	return reconcileProfile(ctx, s.repo, userID, s.cfg, s.now())
}

func reconcileProfile(
	ctx context.Context,
	repo moderationrepo.Repository,
	userID uint64,
	cfg config.ModerationGovernanceConfig,
	now time.Time,
) (UserModerationProfile, error) {
	stored, err := repo.LoadModerationProfile(ctx, userID, now)
	if err != nil {
		return UserModerationProfile{}, err
	}
	profile := serviceProfile(stored)
	evaluated := EvaluateTrust(profile, cfg, now)
	if evaluated.TrustLevel == profile.TrustLevel && sameTime(evaluated.RestrictedUntil, profile.RestrictedUntil) {
		return evaluated, nil
	}
	changed, err := repo.SetAutomaticTrust(ctx, moderationrepo.AutomaticTrustCommand{
		UserID: userID, TrustLevel: moderationrepo.TrustLevel(evaluated.TrustLevel),
		RestrictedUntil: evaluated.RestrictedUntil, UpdatedAt: now,
	})
	if err != nil {
		return UserModerationProfile{}, err
	}
	if changed {
		evaluated.TrustSource = TrustSourceAuto
		evaluated.UpdatedAt = now
	}
	return evaluated, nil
}

func governanceConfigured(cfg config.ModerationGovernanceConfig) bool {
	return cfg.RestrictedScoreThreshold > 0 && cfg.RestrictedDuration > 0
}

// SetTrust 保存管理员校正；手工锁定后自动规则不会覆盖该等级。
func (s *governanceService) SetTrust(ctx context.Context, cmd SetTrustCommand) error {
	if cmd.UserID == 0 || cmd.ActorID == 0 || !validServiceTrust(cmd.TrustLevel) {
		return ErrInvalidRequest
	}
	if cmd.TrustLevel != TrustRestricted {
		cmd.RestrictedUntil = nil
	}
	now := s.now()
	if err := s.repo.EnsureNewProfile(ctx, cmd.UserID, now); err != nil {
		return err
	}
	return s.repo.SetTrust(ctx, moderationrepo.SetTrustCommand{
		UserID: cmd.UserID, ActorID: cmd.ActorID, TrustLevel: moderationrepo.TrustLevel(cmd.TrustLevel),
		ManualLocked: cmd.ManualLocked, RestrictedUntil: cmd.RestrictedUntil, UpdatedAt: now,
	})
}

// SetSanction 设置禁言或封禁；过去的截止时间会被拒绝。
func (s *governanceService) SetSanction(ctx context.Context, cmd SetSanctionCommand) error {
	now := s.now()
	if cmd.UserID == 0 || cmd.ActorID == 0 || (cmd.State != SanctionMuted && cmd.State != SanctionBanned) ||
		(cmd.Until != nil && !cmd.Until.After(now)) {
		return ErrInvalidRequest
	}
	if err := s.repo.EnsureNewProfile(ctx, cmd.UserID, now); err != nil {
		return err
	}
	reason := strings.TrimSpace(cmd.Reason)
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	return s.repo.SetSanction(ctx, moderationrepo.SetSanctionCommand{
		UserID: cmd.UserID, ActorID: cmd.ActorID, State: moderationrepo.SanctionState(cmd.State),
		Until: cmd.Until, Reason: reasonPtr, Now: now,
	})
}

// ReleaseSanction 立即解除用户处罚。
func (s *governanceService) ReleaseSanction(ctx context.Context, userID, actorID uint64) error {
	if userID == 0 || actorID == 0 {
		return ErrInvalidRequest
	}
	return s.repo.ReleaseSanction(ctx, moderationrepo.ReleaseSanctionCommand{UserID: userID, ActorID: actorID, Now: s.now()})
}

// EvaluateTrust 根据当前画像和配置计算自动信任等级；管理员锁定的等级保持不变。
func EvaluateTrust(profile UserModerationProfile, cfg config.ModerationGovernanceConfig, now time.Time) UserModerationProfile {
	if profile.ManualTrustLocked {
		return profile
	}

	// 违规分达到门槛时优先限制，并从最近一次违规开始计算限制期限。
	if profile.ViolationScore >= int64(cfg.RestrictedScoreThreshold) {
		profile.TrustLevel = TrustRestricted
		base := now
		if profile.LastViolationAt != nil {
			base = *profile.LastViolationAt
		}
		until := base.Add(cfg.RestrictedDuration)
		if profile.RestrictedUntil == nil || profile.RestrictedUntil.Before(until) {
			profile.RestrictedUntil = &until
		}
		return profile
	}

	// 只有无违规分的连续干净审核才参与自动晋升。
	if profile.ViolationScore != 0 {
		return profile
	}
	accountAge := now.Sub(profile.CreatedAt)
	switch profile.TrustLevel {
	case TrustNew:
		if meetsPromotion(profile.CleanApprovalStreak, accountAge, cfg.NewToNormal) {
			profile.TrustLevel = TrustNormal
		}
	case TrustNormal:
		if meetsPromotion(profile.CleanApprovalStreak, accountAge, cfg.NormalToTrusted) {
			profile.TrustLevel = TrustTrusted
		}
	}
	return profile
}

func meetsPromotion(cleanApprovals uint64, age time.Duration, threshold config.ModerationPromotionConfig) bool {
	return cleanApprovals >= uint64(threshold.CleanApprovals) && age >= time.Duration(threshold.MinAgeDays)*24*time.Hour
}

func serviceProfile(profile moderationrepo.ModerationProfile) UserModerationProfile {
	return UserModerationProfile{
		UserID: profile.UserID, TrustLevel: TrustLevel(profile.TrustLevel), TrustSource: string(profile.TrustSource),
		ManualTrustLocked: profile.ManualTrustLocked, SanctionState: SanctionState(profile.SanctionState),
		SanctionUntil: profile.SanctionUntil, SanctionReason: profile.SanctionReason,
		CleanApprovalStreak: profile.CleanApprovalStreak, CorrectedCount: profile.CorrectedCount,
		RejectedCount: profile.RejectedCount, HighRiskCount: profile.HighRiskCount,
		ViolationScore: profile.ViolationScore, LastViolationAt: profile.LastViolationAt,
		RestrictedUntil: profile.RestrictedUntil, CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt,
	}
}

func validServiceTrust(level TrustLevel) bool {
	return level == TrustNew || level == TrustNormal || level == TrustTrusted || level == TrustRestricted
}

func sameTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
