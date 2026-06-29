package moderation

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/pkg/config"
)

// RegistrationMode 是全站注册控制状态。
type RegistrationMode = moderationrepo.RegistrationMode

const (
	RegistrationOpen   = moderationrepo.RegistrationOpen
	RegistrationClosed = moderationrepo.RegistrationClosed
)

// Control 是管理端读取的全站审核控制投影。
type Control struct {
	RegistrationMode RegistrationMode
	PublishingMode   PublishingMode
	Reason           *string
	OperatorID       *uint64
	ChangedAt        time.Time
	LockVersion      uint64
}

// UpdateControlCommand 修改全站注册和发布状态。
type UpdateControlCommand struct {
	RegistrationMode    RegistrationMode
	PublishingMode      PublishingMode
	Reason              string
	OperatorID          uint64
	ExpectedLockVersion uint64
}

// EmergencyItemCommand 隐藏或恢复单个审核项。
type EmergencyItemCommand struct {
	ItemID  uint64
	ActorID uint64
	Reason  string
}

// EmergencyItemResult 返回紧急操作后的公开状态。
type EmergencyItemResult struct {
	ItemID      uint64
	PublicState PublicState
	LockVersion uint64
}

// UserEmergencyBatchCommand 分批隐藏或恢复用户内容。
type UserEmergencyBatchCommand struct {
	UserID  uint64
	ActorID uint64
	Cursor  uint64
	Limit   int
	Reason  string
}

// EmergencyBatchResult 返回批量操作的游标进度。
type EmergencyBatchResult = moderationrepo.EmergencyBatchResult

// OperationsService 提供管理员全站控制和紧急内容处置。
type OperationsService interface {
	GetControl(ctx context.Context) (Control, error)
	UpdateControl(ctx context.Context, cmd UpdateControlCommand) (Control, error)
	HideItem(ctx context.Context, cmd EmergencyItemCommand) (EmergencyItemResult, error)
	RestoreItem(ctx context.Context, cmd EmergencyItemCommand) (EmergencyItemResult, error)
	HideUserContent(ctx context.Context, cmd UserEmergencyBatchCommand) (EmergencyBatchResult, error)
	RestoreUserContent(ctx context.Context, cmd UserEmergencyBatchCommand) (EmergencyBatchResult, error)
	GetUserProfile(ctx context.Context, userID uint64) (UserModerationProfile, error)
	SetUserTrust(ctx context.Context, cmd SetTrustCommand) error
	SetUserSanction(ctx context.Context, cmd SetSanctionCommand) error
	ReleaseUserSanction(ctx context.Context, userID, actorID uint64) error
}

type operationsService struct {
	repo       moderationrepo.Repository
	governance GovernanceService
	cfg        config.ModerationConfig
	now        func() time.Time
}

// NewOperationsService 通过构造注入创建管理端审核运维服务。
func NewOperationsService(
	repo moderationrepo.Repository,
	governance GovernanceService,
	cfg config.ModerationConfig,
	now func() time.Time,
) OperationsService {
	if now == nil {
		now = time.Now
	}
	return &operationsService{repo: repo, governance: governance, cfg: cfg, now: now}
}

// GetControl 读取全站审核控制。
func (s *operationsService) GetControl(ctx context.Context) (Control, error) {
	record, err := s.repo.LoadControl(ctx)
	if err != nil {
		return Control{}, err
	}
	return controlFromRecord(record), nil
}

// UpdateControl 校验配置边界并使用乐观锁保存控制状态。
func (s *operationsService) UpdateControl(ctx context.Context, cmd UpdateControlCommand) (Control, error) {
	if cmd.OperatorID == 0 || cmd.ExpectedLockVersion == 0 ||
		(cmd.RegistrationMode != RegistrationOpen && cmd.RegistrationMode != RegistrationClosed) ||
		(cmd.PublishingMode != PublishingOpen && cmd.PublishingMode != PublishingPreReviewAll && cmd.PublishingMode != PublishingClosed) {
		return Control{}, ErrInvalidRequest
	}
	reason, err := s.optionalReason(cmd.Reason)
	if err != nil {
		return Control{}, err
	}
	now := s.now()
	if err := s.repo.UpdateControl(ctx, moderationrepo.UpdateControlCommand{
		RegistrationMode: moderationrepo.RegistrationMode(cmd.RegistrationMode),
		PublishingMode:   moderationrepo.PublishingMode(cmd.PublishingMode), Reason: reason,
		OperatorID: cmd.OperatorID, ExpectedLockVersion: cmd.ExpectedLockVersion, ChangedAt: now,
	}); err != nil {
		if err == moderationrepo.ErrOptimisticLock {
			return Control{}, ErrReviewConflict
		}
		return Control{}, err
	}
	operatorID := cmd.OperatorID
	return Control{
		RegistrationMode: cmd.RegistrationMode, PublishingMode: cmd.PublishingMode,
		Reason: reason, OperatorID: &operatorID, ChangedAt: now, LockVersion: cmd.ExpectedLockVersion + 1,
	}, nil
}

// HideItem 紧急隐藏一个当前已通过且公开的审核项。
func (s *operationsService) HideItem(ctx context.Context, cmd EmergencyItemCommand) (EmergencyItemResult, error) {
	return s.applyItemEmergency(ctx, cmd, EventEmergencyHide)
}

// RestoreItem 仅恢复紧急隐藏前保存的公开状态；删除态不可恢复。
func (s *operationsService) RestoreItem(ctx context.Context, cmd EmergencyItemCommand) (EmergencyItemResult, error) {
	return s.applyItemEmergency(ctx, cmd, EventRestore)
}

func (s *operationsService) applyItemEmergency(ctx context.Context, cmd EmergencyItemCommand, event Event) (EmergencyItemResult, error) {
	if cmd.ItemID == 0 || cmd.ActorID == 0 {
		return EmergencyItemResult{}, ErrInvalidRequest
	}
	reason, err := s.optionalReason(cmd.Reason)
	if err != nil || (event == EventEmergencyHide && reason == nil) {
		return EmergencyItemResult{}, ErrInvalidRequest
	}
	record, err := s.repo.LoadCurrentReviewRecord(ctx, cmd.ItemID)
	if err != nil {
		return EmergencyItemResult{}, mapReviewRepositoryError(err)
	}
	now := s.now()
	plan, err := Transition(TransitionInput{
		Previous: itemSnapshot(record.State), Event: event, Reason: valueOrEmpty(reason), Now: now,
	})
	if err != nil {
		return EmergencyItemResult{}, err
	}
	actorID, subjectID := cmd.ActorID, record.AuthorID
	applied, err := s.repo.ApplyTransition(ctx, moderationrepo.ApplyTransitionCommand{
		Subject: record.Subject, AuthorID: record.AuthorID, ExpectedLockVersion: record.LockVersion,
		Next: itemState(plan.Item),
		Log: &moderationrepo.ActionLog{
			ActorUserID: &actorID, SubjectUserID: &subjectID, Action: moderationrepo.Event(event),
			Reason: reason, CreatedAt: now,
		},
	})
	if err != nil {
		return EmergencyItemResult{}, mapReviewRepositoryError(err)
	}
	return EmergencyItemResult{ItemID: record.ItemID, PublicState: plan.Item.PublicState, LockVersion: applied.LockVersion}, nil
}

// HideUserContent 分批隔离一个用户所有符合条件的公开内容。
func (s *operationsService) HideUserContent(ctx context.Context, cmd UserEmergencyBatchCommand) (EmergencyBatchResult, error) {
	return s.applyUserEmergency(ctx, cmd, true)
}

// RestoreUserContent 分批恢复该用户由紧急操作隔离的内容。
func (s *operationsService) RestoreUserContent(ctx context.Context, cmd UserEmergencyBatchCommand) (EmergencyBatchResult, error) {
	return s.applyUserEmergency(ctx, cmd, false)
}

func (s *operationsService) applyUserEmergency(ctx context.Context, cmd UserEmergencyBatchCommand, hide bool) (EmergencyBatchResult, error) {
	if cmd.UserID == 0 || cmd.ActorID == 0 || cmd.Limit < 0 || cmd.Limit > s.cfg.Control.UserHideMaxItemsPerRequest {
		return EmergencyBatchResult{}, ErrInvalidRequest
	}
	reason, err := s.optionalReason(cmd.Reason)
	if err != nil || (hide && reason == nil) {
		return EmergencyBatchResult{}, ErrInvalidRequest
	}
	limit := cmd.Limit
	if limit == 0 || limit > s.cfg.Control.UserHideBatchSize {
		limit = s.cfg.Control.UserHideBatchSize
	}
	return s.repo.ApplyUserEmergencyBatch(ctx, moderationrepo.UserEmergencyBatchCommand{
		UserID: cmd.UserID, ActorID: cmd.ActorID, Cursor: cmd.Cursor, Limit: limit,
		Hide: hide, Reason: reason, Now: s.now(),
	})
}

// GetUserProfile 读取并刷新用户审核画像。
func (s *operationsService) GetUserProfile(ctx context.Context, userID uint64) (UserModerationProfile, error) {
	if s.governance == nil {
		return UserModerationProfile{}, ErrInvalidRequest
	}
	return s.governance.GetProfile(ctx, userID)
}

// SetUserTrust 保存管理员手工信任等级。
func (s *operationsService) SetUserTrust(ctx context.Context, cmd SetTrustCommand) error {
	if s.governance == nil {
		return ErrInvalidRequest
	}
	return s.governance.SetTrust(ctx, cmd)
}

// SetUserSanction 设置管理员处罚。
func (s *operationsService) SetUserSanction(ctx context.Context, cmd SetSanctionCommand) error {
	if s.governance == nil {
		return ErrInvalidRequest
	}
	return s.governance.SetSanction(ctx, cmd)
}

// ReleaseUserSanction 解除管理员处罚。
func (s *operationsService) ReleaseUserSanction(ctx context.Context, userID, actorID uint64) error {
	if s.governance == nil {
		return ErrInvalidRequest
	}
	return s.governance.ReleaseSanction(ctx, userID, actorID)
}

func (s *operationsService) optionalReason(value string) (*string, error) {
	trimmed := strings.TrimSpace(value)
	if utf8.RuneCountInString(trimmed) > s.cfg.Review.ReasonMaxChars {
		return nil, ErrInvalidRequest
	}
	if trimmed == "" {
		return nil, nil
	}
	return &trimmed, nil
}

func controlFromRecord(record moderationrepo.ControlRecord) Control {
	return Control{
		RegistrationMode: record.RegistrationMode, PublishingMode: PublishingMode(record.PublishingMode),
		Reason: record.Reason, OperatorID: record.OperatorID, ChangedAt: record.ChangedAt, LockVersion: record.LockVersion,
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
