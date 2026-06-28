package moderation

import (
	"context"
	"time"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
)

// SubjectType 是业务内容的封闭审核类型。
type SubjectType = moderationrepo.SubjectType

const (
	SubjectMoment              = moderationrepo.SubjectMoment
	SubjectArticleComment      = moderationrepo.SubjectArticleComment
	SubjectMomentComment       = moderationrepo.SubjectMomentComment
	SubjectGuestbook           = moderationrepo.SubjectGuestbook
	SubjectArticleCommentReply = moderationrepo.SubjectArticleCommentReply
	SubjectMomentCommentReply  = moderationrepo.SubjectMomentCommentReply
	SubjectGuestbookReply      = moderationrepo.SubjectGuestbookReply
)

// SubjectRef 定位业务内容并携带创建或校验父关系所需信息。
type SubjectRef = moderationrepo.SubjectRef

// SubjectKey 是审核视图使用的稳定值键。
type SubjectKey = moderationrepo.SubjectKey

// Viewer 与 View 是业务 service 可读取的内部审核投影。
type Viewer = moderationrepo.Viewer
type View = moderationrepo.View

// PolicyDecider 将风险和用户上下文映射为审核动作。
type PolicyDecider interface {
	Decide(input PolicyInput) (PolicyAction, error)
}

type defaultPolicyDecider struct{}

// NewPolicyDecider 创建使用固定优先级规则的默认策略器。
func NewPolicyDecider() PolicyDecider { return defaultPolicyDecider{} }

func (defaultPolicyDecider) Decide(input PolicyInput) (PolicyAction, error) { return Decide(input) }

// SubmitCommand 是首次发布内容的统一审核命令。
type SubmitCommand struct {
	ActorID        uint64
	IsAdmin        bool
	Subject        SubjectRef
	Content        string
	ImageKeys      []string
	IdempotencyKey string
}

// EditCommand 是编辑已有内容的统一审核命令。
type EditCommand struct {
	ActorID        uint64
	IsAdmin        bool
	Subject        SubjectRef
	Content        string
	ImageKeys      []string
	IdempotencyKey string
}

// DeleteCommand 是删除内容的统一审核命令。
type DeleteCommand struct {
	ActorID uint64
	IsAdmin bool
	Subject SubjectRef
	Reason  string
}

// SubmitResult 是业务 service 和 handler 使用的安全审核结果。
type SubmitResult struct {
	Subject            SubjectRef
	ItemID             uint64
	RevisionID         uint64
	RevisionVersion    uint64
	LockVersion        uint64
	RiskLevel          RiskLevel
	Action             PolicyAction
	PublicState        PublicState
	ReviewStatus       ReviewStatus
	Content            string
	Message            string
	HasPendingRevision bool
	CanInteract        bool
}

// Service 是评论、留言和碎语业务依赖的审核门面。
type Service interface {
	Submit(ctx context.Context, cmd SubmitCommand) (SubmitResult, error)
	Edit(ctx context.Context, cmd EditCommand) (SubmitResult, error)
	Delete(ctx context.Context, cmd DeleteCommand) error
	AssertCanInteract(ctx context.Context, ref SubjectRef) error
	LoadViews(ctx context.Context, refs []SubjectRef, viewer Viewer) (map[SubjectKey]View, error)
}

type applicationService struct {
	repo       moderationrepo.Repository
	processor  ContentProcessor
	classifier Classifier
	decider    PolicyDecider
	cfg        config.ModerationConfig
	logger     *zap.Logger
	now        func() time.Time
}

// NewService 通过构造注入创建审核应用服务。
func NewService(
	repo moderationrepo.Repository,
	processor ContentProcessor,
	classifier Classifier,
	decider PolicyDecider,
	cfg config.ModerationConfig,
	logger *zap.Logger,
	now func() time.Time,
) Service {
	if processor == nil {
		processor = NewContentProcessor()
	}
	if decider == nil {
		decider = NewPolicyDecider()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if now == nil {
		now = time.Now
	}
	return &applicationService{
		repo: repo, processor: processor, classifier: classifier, decider: decider,
		cfg: cfg, logger: logger, now: now,
	}
}
