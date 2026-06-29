package moderation

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Repository 是 service 可 mock 的审核数据边界。
type Repository interface {
	LoadSubject(ctx context.Context, ref SubjectRef) (SubjectSnapshot, error)
	LoadItemState(ctx context.Context, ref SubjectRef) (ItemStateRecord, error)
	LoadPolicyContext(ctx context.Context, userID uint64) (PolicyContext, error)
	EnsureNewProfile(ctx context.Context, userID uint64, now time.Time) error
	LoadModerationProfile(ctx context.Context, userID uint64, now time.Time) (ModerationProfile, error)
	SetAutomaticTrust(ctx context.Context, cmd AutomaticTrustCommand) (bool, error)
	SetTrust(ctx context.Context, cmd SetTrustCommand) error
	SetSanction(ctx context.Context, cmd SetSanctionCommand) error
	ReleaseSanction(ctx context.Context, userID uint64, now time.Time) error
	FindResultByIdempotencyKey(ctx context.Context, userID uint64, key string) (*StoredResult, error)
	ApplyTransition(ctx context.Context, cmd ApplyTransitionCommand) (AppliedTransition, error)
	RecordBlockedAttempt(ctx context.Context, attempt BlockedAttempt) (StoredResult, error)
	LoadEnabledRules(ctx context.Context) ([]RuleRecord, error)
	LoadModerationView(ctx context.Context, refs []SubjectRef, viewer Viewer) (map[SubjectKey]View, error)
	ListReviewRecords(ctx context.Context, filter ReviewFilter) (ReviewPage, error)
	LoadReviewRecord(ctx context.Context, itemID, revisionID uint64) (ReviewRecord, error)
	LoadCurrentReviewRecord(ctx context.Context, itemID uint64) (ReviewRecord, error)
	UseApprovedImage(ctx context.Context, fingerprint ImageFingerprint, usedAt time.Time) (bool, error)
	UpsertPendingImage(ctx context.Context, image PendingImage) error
	LoadRevisionImages(ctx context.Context, revisionID uint64) ([]RevisionImageRecord, error)
	LoadRevisionPreviewKeys(ctx context.Context, revisionID uint64) ([]string, error)
}

type repository struct {
	db       *gorm.DB
	adapters map[SubjectType]subjectAdapter
}

// NewRepository 构造审核仓储，数据库连接必须由调用方注入。
func NewRepository(db *gorm.DB) Repository {
	comment := &commentAdapter{}
	guestbook := &guestbookAdapter{}
	moment := &momentAdapter{}
	return &repository{
		db: db,
		adapters: map[SubjectType]subjectAdapter{
			SubjectArticleComment:      comment,
			SubjectMomentComment:       comment,
			SubjectArticleCommentReply: comment,
			SubjectMomentCommentReply:  comment,
			SubjectGuestbook:           guestbook,
			SubjectGuestbookReply:      guestbook,
			SubjectMoment:              moment,
		},
	}
}

type subjectAdapter interface {
	Load(ctx context.Context, db *gorm.DB, ref SubjectRef) (SubjectSnapshot, error)
	Lock(ctx context.Context, tx *gorm.DB, ref SubjectRef, authorID uint64) (SubjectSnapshot, error)
	Materialize(ctx context.Context, tx *gorm.DB, cmd MaterializeCommand) error
	Delete(ctx context.Context, tx *gorm.DB, ref SubjectRef) error
	Descendants(ctx context.Context, tx *gorm.DB, ref SubjectRef) ([]SubjectRef, error)
}

func (r *repository) adapter(subjectType SubjectType) (subjectAdapter, error) {
	adapter, ok := r.adapters[subjectType]
	if !ok {
		return nil, ErrInvalidCommand
	}
	return adapter, nil
}
