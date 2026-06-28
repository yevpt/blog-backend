package moderation

import (
	"context"

	"gorm.io/gorm"
)

// Repository 是 service 可 mock 的审核数据边界。
type Repository interface {
	LoadSubject(ctx context.Context, ref SubjectRef) (SubjectSnapshot, error)
	LoadItemState(ctx context.Context, ref SubjectRef) (ItemStateRecord, error)
	LoadPolicyContext(ctx context.Context, userID uint64) (PolicyContext, error)
	FindResultByIdempotencyKey(ctx context.Context, userID uint64, key string) (*StoredResult, error)
	ApplyTransition(ctx context.Context, cmd ApplyTransitionCommand) (AppliedTransition, error)
	RecordBlockedAttempt(ctx context.Context, attempt BlockedAttempt) (StoredResult, error)
	LoadEnabledRules(ctx context.Context) ([]RuleRecord, error)
	LoadModerationView(ctx context.Context, refs []SubjectRef, viewer Viewer) (map[SubjectKey]View, error)
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
