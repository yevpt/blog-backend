package moderation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
)

// ListReviewCommand 是管理员审核列表的筛选条件。
type ListReviewCommand struct {
	Page                     int
	PageSize                 int
	ContentType              *SubjectType
	RiskLevel                *RiskLevel
	ReviewStatus             *ReviewStatus
	IncludeAllReviewStatuses bool
	PublicState              *PublicState
}

// ReviewCommand 定位一次基于明确待审版本和锁版本的人工决策。
type ReviewCommand struct {
	ItemID              uint64
	RevisionID          uint64
	ExpectedLockVersion uint64
	ReviewerID          uint64
	Reason              string
}

// CorrectCommand 是管理员修正正文后通过的命令。
type CorrectCommand struct {
	ReviewCommand
	Content string
}

// ReviewItem 是 service 层稳定的审核版本投影。
type ReviewItem struct {
	ItemID           uint64
	Subject          SubjectRef
	AuthorID         uint64
	LockVersion      uint64
	LifecycleState   LifecycleState
	PublicState      PublicState
	RevisionID       uint64
	RevisionVersion  uint64
	SubmittedContent string
	PublishedContent string
	RiskLevel        RiskLevel
	PolicyAction     PolicyAction
	ReviewStatus     ReviewStatus
	MomentOptions    *MomentOptions
	DecisionType     *string
	DecisionReason   *string
	ReviewerID       *uint64
	ReviewedAt       *time.Time
	// EmergencyHideReason 是紧急隐藏原因，仅紧急隐藏态有值。
	EmergencyHideReason *string
	// EmergencyHiddenAt 是紧急隐藏发生时间，仅紧急隐藏态有值。
	EmergencyHiddenAt *time.Time
	CreatedAt         time.Time
	CanInteract       bool
}

// ReviewPage 是人工审核列表分页结果。
type ReviewPage struct {
	Total    int64
	Page     int
	PageSize int
	Items    []ReviewItem
}

// ReviewService 提供管理员审核查询、通过、修正和驳回用例。
type ReviewService interface {
	List(ctx context.Context, cmd ListReviewCommand) (ReviewPage, error)
	Get(ctx context.Context, itemID uint64) (ReviewItem, error)
	History(ctx context.Context, cmd ReviewHistoryCommand) (ReviewHistoryPage, error)
	Approve(ctx context.Context, cmd ReviewCommand) (ReviewItem, error)
	Correct(ctx context.Context, cmd CorrectCommand) (ReviewItem, error)
	Reject(ctx context.Context, cmd ReviewCommand) (ReviewItem, error)
}

// PreviewCleaner 在审核事务提交后删除不再需要的独占低清预览。
type PreviewCleaner interface {
	DeletePreviewObjects(ctx context.Context, keys []string) error
}

type reviewService struct {
	repo      moderationrepo.Repository
	processor ContentProcessor
	cleaner   PreviewCleaner
	publisher ApprovedImagePublisher
	cfg       config.ModerationConfig
	logger    *zap.Logger
	now       func() time.Time
}

// NewReviewService 通过构造注入创建人工审核服务。
func NewReviewService(
	repo moderationrepo.Repository,
	processor ContentProcessor,
	cleaner PreviewCleaner,
	publisher ApprovedImagePublisher,
	cfg config.ModerationConfig,
	logger *zap.Logger,
	now func() time.Time,
) ReviewService {
	if logger == nil {
		logger = zap.NewNop()
	}
	if now == nil {
		now = time.Now
	}
	return &reviewService{repo: repo, processor: processor, cleaner: cleaner, publisher: publisher, cfg: cfg, logger: logger, now: now}
}

// List 分页查询审核版本；未指定状态时默认只返回待审队列。
func (s *reviewService) List(ctx context.Context, cmd ListReviewCommand) (ReviewPage, error) {
	page, pageSize := s.normalizeReviewPage(cmd.Page, cmd.PageSize)
	filter := moderationrepo.ReviewFilter{Page: page, PageSize: pageSize}
	if !cmd.IncludeAllReviewStatuses {
		status := ReviewPending
		if cmd.ReviewStatus != nil {
			status = *cmd.ReviewStatus
		}
		repoStatus := moderationrepo.ReviewStatus(status)
		filter.ReviewStatus = &repoStatus
	}
	if cmd.ContentType != nil {
		value := moderationrepo.SubjectType(*cmd.ContentType)
		filter.ContentType = &value
	}
	if cmd.RiskLevel != nil {
		value := moderationrepo.RiskLevel(*cmd.RiskLevel)
		filter.RiskLevel = &value
	}
	if cmd.PublicState != nil {
		value := moderationrepo.PublicState(*cmd.PublicState)
		filter.PublicState = &value
	}
	result, err := s.repo.ListReviewRecords(ctx, filter)
	if err != nil {
		return ReviewPage{}, mapReviewRepositoryError(err)
	}
	items := make([]ReviewItem, 0, len(result.Items))
	for _, record := range result.Items {
		items = append(items, reviewItemFromRecord(record))
	}
	return ReviewPage{Total: result.Total, Page: page, PageSize: pageSize, Items: items}, nil
}

// Get 返回审核项当前待审版本；没有待审版本时返回最新历史版本。
func (s *reviewService) Get(ctx context.Context, itemID uint64) (ReviewItem, error) {
	if itemID == 0 || s.repo == nil {
		return ReviewItem{}, ErrInvalidRequest
	}
	record, err := s.repo.LoadCurrentReviewRecord(ctx, itemID)
	if err != nil {
		return ReviewItem{}, mapReviewRepositoryError(err)
	}
	return reviewItemFromRecord(record), nil
}

// Approve 通过当前待审版本。
func (s *reviewService) Approve(ctx context.Context, cmd ReviewCommand) (ReviewItem, error) {
	return s.review(ctx, EventApprove, cmd, nil)
}

// Correct 清洗管理员修正文并作为正式版本通过。
func (s *reviewService) Correct(ctx context.Context, cmd CorrectCommand) (ReviewItem, error) {
	if strings.TrimSpace(cmd.Content) == "" {
		return ReviewItem{}, ErrInvalidRequest
	}
	if err := s.validateReviewCommand(cmd.ReviewCommand, true); err != nil {
		return ReviewItem{}, err
	}
	record, err := s.repo.LoadReviewRecord(ctx, cmd.ItemID, cmd.RevisionID)
	if err != nil {
		return ReviewItem{}, mapReviewRepositoryError(err)
	}
	processed, err := s.processor.Process(cmd.Content, reviewContentLimit(s.cfg.Content, record.Subject.Type))
	if err != nil {
		return ReviewItem{}, err
	}
	if strings.TrimSpace(processed.PlainText) == "" {
		return ReviewItem{}, fmt.Errorf("%w: corrected content is empty after sanitization", ErrInvalidRequest)
	}
	return s.applyReview(ctx, EventCorrectAndApprove, cmd.ReviewCommand, record, &processed.Published)
}

// Reject 驳回当前待审版本并按状态机恢复最后通过版本。
func (s *reviewService) Reject(ctx context.Context, cmd ReviewCommand) (ReviewItem, error) {
	return s.review(ctx, EventReject, cmd, nil)
}

func (s *reviewService) review(ctx context.Context, event Event, cmd ReviewCommand, corrected *string) (ReviewItem, error) {
	reasonRequired := event == EventReject || event == EventCorrectAndApprove
	if err := s.validateReviewCommand(cmd, reasonRequired); err != nil {
		return ReviewItem{}, err
	}
	record, err := s.repo.LoadReviewRecord(ctx, cmd.ItemID, cmd.RevisionID)
	if err != nil {
		return ReviewItem{}, mapReviewRepositoryError(err)
	}
	return s.applyReview(ctx, event, cmd, record, corrected)
}

func (s *reviewService) applyReview(
	ctx context.Context,
	event Event,
	cmd ReviewCommand,
	record moderationrepo.ReviewRecord,
	corrected *string,
) (ReviewItem, error) {
	if record.State.LifecycleState == moderationrepo.LifecycleDeleted {
		return ReviewItem{}, ErrAlreadyDeleted
	}
	if record.LockVersion != cmd.ExpectedLockVersion || record.ReviewStatus != moderationrepo.ReviewPending {
		return ReviewItem{}, ErrReviewConflict
	}
	now := s.now()
	var previewKeys []string
	if s.cleaner != nil && (event == EventApprove || event == EventCorrectAndApprove) {
		var loadErr error
		previewKeys, loadErr = s.repo.LoadRevisionPreviewKeys(ctx, record.RevisionID)
		if loadErr != nil {
			return ReviewItem{}, mapReviewRepositoryError(loadErr)
		}
	}
	// 事务前读取当前和旧图片，用于事务后正式化。
	publishCurrent, publishPrevious, publishErr := s.loadMomentPublishImages(ctx, record, event)
	if publishErr != nil {
		return ReviewItem{}, mapReviewRepositoryError(publishErr)
	}
	plan, err := Transition(TransitionInput{
		Event: event, Previous: itemSnapshot(record.State), NewRevisionID: record.RevisionID,
		Reason: strings.TrimSpace(cmd.Reason), Now: now,
	})
	if err != nil {
		return ReviewItem{}, err
	}
	persisted := buildReviewTransition(record, cmd, event, corrected, plan, now, s.cfg, s.loadReviewNotificationContext(ctx, record))
	applied, err := s.repo.ApplyTransition(ctx, persisted)
	if err != nil {
		return ReviewItem{}, mapReviewRepositoryError(err)
	}
	if governanceConfigured(s.cfg.Governance) {
		if _, governanceErr := reconcileProfile(ctx, s.repo, record.AuthorID, s.cfg.Governance, now); governanceErr != nil {
			s.logger.Warn("刷新用户审核画像失败，将在下次访问时重试",
				zap.Uint64("user_id", record.AuthorID), zap.Error(governanceErr))
		}
	}
	if len(previewKeys) > 0 {
		if cleanupErr := s.cleaner.DeletePreviewObjects(ctx, previewKeys); cleanupErr != nil {
			s.logger.Warn("删除已通过图片预览失败，等待定期清理补偿", zap.Error(cleanupErr))
		}
	}
	if publishErr := s.publishMomentImages(ctx, record, publishCurrent, publishPrevious); publishErr != nil {
		return ReviewItem{}, publishErr
	}
	return appliedReviewItem(record, cmd, corrected, plan, applied.LockVersion, now), nil
}

func (s *reviewService) validateReviewCommand(cmd ReviewCommand, reasonRequired bool) error {
	if s.repo == nil || s.processor == nil || cmd.ItemID == 0 || cmd.RevisionID == 0 ||
		cmd.ExpectedLockVersion == 0 || cmd.ReviewerID == 0 {
		return ErrInvalidRequest
	}
	reason := strings.TrimSpace(cmd.Reason)
	if reasonRequired && reason == "" {
		return fmt.Errorf("%w: review reason is required", ErrInvalidRequest)
	}
	if utf8.RuneCountInString(reason) > s.cfg.Review.ReasonMaxChars {
		return fmt.Errorf("%w: review reason is too long", ErrInvalidRequest)
	}
	return nil
}

func (s *reviewService) normalizeReviewPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = s.cfg.Review.QueueDefaultPageSize
	}
	if pageSize > s.cfg.Review.QueueMaxPageSize {
		pageSize = s.cfg.Review.QueueMaxPageSize
	}
	return page, pageSize
}

func reviewContentLimit(content config.ModerationContentConfig, subjectType SubjectType) int {
	switch subjectType {
	case SubjectMoment:
		return content.MomentMaxChars
	case SubjectArticleComment, SubjectMomentComment:
		return content.CommentMaxChars
	case SubjectGuestbook:
		return content.GuestbookMaxChars
	default:
		return content.ReplyMaxChars
	}
}

func mapReviewRepositoryError(err error) error {
	if errors.Is(err, moderationrepo.ErrOptimisticLock) || errors.Is(err, moderationrepo.ErrPendingRevisionConflict) ||
		errors.Is(err, moderationrepo.ErrRevisionStateConflict) {
		return ErrReviewConflict
	}
	return err
}

// loadMomentPublishImages 事务前读取碎语当前版本和旧物化版本图片，供事务后正式化。
func (s *reviewService) loadMomentPublishImages(
	ctx context.Context,
	record moderationrepo.ReviewRecord,
	event Event,
) ([]moderationrepo.RevisionImageRecord, []moderationrepo.RevisionImageRecord, error) {
	if s.publisher == nil || (event != EventApprove && event != EventCorrectAndApprove) ||
		record.Subject.Type != moderationrepo.SubjectMoment {
		return nil, nil, nil
	}
	current, err := s.repo.LoadRevisionImages(ctx, record.RevisionID)
	if err != nil {
		return nil, nil, err
	}
	var previous []moderationrepo.RevisionImageRecord
	if record.State.Materialized.ID != 0 {
		previous, err = s.repo.LoadRevisionImages(ctx, record.State.Materialized.ID)
		if err != nil {
			return nil, nil, err
		}
	}
	return current, previous, nil
}

// publishMomentImages 事务提交后正式化碎语图片，失败时收回公开投影并返回错误。
func (s *reviewService) publishMomentImages(
	ctx context.Context,
	record moderationrepo.ReviewRecord,
	current, previous []moderationrepo.RevisionImageRecord,
) error {
	if s.publisher == nil || (len(current) == 0 && len(previous) == 0) {
		return nil
	}
	_, err := s.publisher.Publish(ctx, moderationmedia.PublishCommand{
		ItemID: record.ItemID, RevisionID: record.RevisionID,
		UserID: record.AuthorID, MomentID: record.Subject.ID,
		Current: current, Previous: previous,
	})
	if err != nil {
		s.logger.Error("碎语图片正式化失败，收回公开投影",
			zap.Uint64("item_id", record.ItemID),
			zap.Uint64("moment_id", record.Subject.ID),
			zap.Error(err),
		)
		if revertErr := s.repo.RevertPublicProjection(ctx, record.ItemID, record.Subject.ID); revertErr != nil {
			s.logger.Error("收回公开投影失败", zap.Error(revertErr))
		}
		return err
	}
	return nil
}
