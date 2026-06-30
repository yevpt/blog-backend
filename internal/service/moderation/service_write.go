package moderation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
)

const newRevisionSentinel = ^uint64(0)
const maxIdempotencyKeyChars = 128

type writeInput struct {
	actorID        uint64
	authorID       uint64
	isAdmin        bool
	subject        SubjectRef
	content        string
	imageKeys      []string
	idempotencyKey string
	momentOptions  *moderationrepo.MomentOptions
	images         []moderationrepo.RevisionImageDraft
	imageViews     []moderationrepo.ImageView
	isEdit         bool
}

func (s *applicationService) Submit(ctx context.Context, cmd SubmitCommand) (SubmitResult, error) {
	return s.write(ctx, writeInput{
		actorID: cmd.ActorID, authorID: cmd.AuthorID, isAdmin: cmd.IsAdmin, subject: cmd.Subject,
		content: cmd.Content, imageKeys: cmd.ImageKeys, idempotencyKey: cmd.IdempotencyKey,
		momentOptions: cloneMomentOptions(cmd.MomentOptions),
	})
}

func (s *applicationService) Edit(ctx context.Context, cmd EditCommand) (SubmitResult, error) {
	return s.write(ctx, writeInput{
		actorID: cmd.ActorID, isAdmin: cmd.IsAdmin, subject: cmd.Subject,
		content: cmd.Content, imageKeys: cmd.ImageKeys, idempotencyKey: cmd.IdempotencyKey,
		momentOptions: cloneMomentOptions(cmd.MomentOptions),
		isEdit:        true,
	})
}

func (s *applicationService) write(ctx context.Context, input writeInput) (SubmitResult, error) {
	if err := s.validateWrite(input); err != nil {
		return SubmitResult{}, err
	}
	stored, err := s.repo.FindResultByIdempotencyKey(ctx, input.actorID, input.idempotencyKey)
	if err != nil {
		return SubmitResult{}, err
	}
	if stored != nil {
		return s.resultFromStored(*stored, input.subject)
	}
	// 每次写入前协调自动等级，补偿先前审核后画像刷新失败，并及时释放到期限制。
	if governanceConfigured(s.cfg.Governance) {
		if _, err := reconcileProfile(ctx, s.repo, input.actorID, s.cfg.Governance, s.now()); err != nil {
			return SubmitResult{}, err
		}
	}

	policyContext, err := s.repo.LoadPolicyContext(ctx, input.actorID)
	if err != nil {
		return SubmitResult{}, err
	}
	if publishingBlocked(input.isAdmin, policyContext) {
		return SubmitResult{}, ErrPublishingForbidden
	}
	processed, err := s.processor.Process(input.content, s.contentLimit(input.subject.Type))
	if err != nil {
		return SubmitResult{}, err
	}
	if strings.TrimSpace(processed.PlainText) == "" {
		return SubmitResult{}, fmt.Errorf("%w: content is empty after sanitization", ErrInvalidRequest)
	}
	if len(processed.Links) > s.cfg.Content.MaxLinksPerContent {
		return SubmitResult{}, fmt.Errorf("%w: too many links", ErrInvalidRequest)
	}
	classification := s.classifier.Classify(processed)
	hasUnapprovedImage := false
	if classification.Risk != RiskHigh && len(input.imageKeys) > 0 {
		if len(input.imageKeys) > s.cfg.Content.MaxImagesPerContent || s.media == nil {
			return SubmitResult{}, ErrImageReviewUnavailable
		}
		authorID := input.actorID
		if input.authorID != 0 {
			authorID = input.authorID
		}
		prepared, prepareErr := s.media.Prepare(ctx, authorID, input.imageKeys)
		if prepareErr != nil {
			if errors.Is(prepareErr, moderationmedia.ErrInvalidImage) {
				return SubmitResult{}, fmt.Errorf("%w: invalid image", ErrInvalidRequest)
			}
			return SubmitResult{}, ErrImageReviewUnavailable
		}
		input.images = revisionImageDrafts(prepared.Images)
		input.imageViews = preparedImageViews(prepared.Images)
		processed.Published = applyImageReplacements(processed.Published, prepared.Replacements)
		for _, image := range prepared.Images {
			if !image.Approved {
				hasUnapprovedImage = true
				break
			}
		}
	}
	action, err := s.decider.Decide(policyInput(input, policyContext, classification, len(processed.Links) > 0, hasUnapprovedImage, s.cfg))
	if err != nil {
		return SubmitResult{}, err
	}

	var item moderationrepo.ItemStateRecord
	var previousContent string
	if input.isEdit {
		item, err = s.repo.LoadItemState(ctx, input.subject)
		if err != nil {
			return SubmitResult{}, err
		}
		if item.State.LifecycleState == moderationrepo.LifecycleDeleted {
			return SubmitResult{}, ErrAlreadyDeleted
		}
		if item.AuthorID != input.actorID && !input.isAdmin {
			return SubmitResult{}, moderationrepo.ErrSubjectNotFound
		}
		subject, loadErr := s.repo.LoadSubject(ctx, input.subject)
		if loadErr != nil {
			return SubmitResult{}, loadErr
		}
		if subject.AuthorID != item.AuthorID {
			return SubmitResult{}, moderationrepo.ErrSubjectNotFound
		}
		input.subject = subject.Ref
		previousContent = subject.Content
	}
	if action == ActionBlock {
		if classification.Risk != RiskHigh || input.isAdmin {
			return SubmitResult{}, ErrPublishingForbidden
		}
		return s.recordRiskBlock(ctx, input, item, classification)
	}

	plan, err := Transition(TransitionInput{
		Event:         chooseSubmitEvent(input.isEdit),
		Action:        action,
		NewRevisionID: newRevisionSentinel,
		Previous:      itemSnapshot(item.State),
		Now:           s.now(),
	})
	if err != nil {
		return SubmitResult{}, err
	}
	command := s.transitionCommand(input, item, processed, classification, action, plan)
	applied, err := s.repo.ApplyTransition(ctx, command)
	if err != nil {
		if errors.Is(err, moderationrepo.ErrIdempotencyDomainConflict) {
			return s.resolveRace(ctx, input)
		}
		return SubmitResult{}, err
	}
	if applied.Replay != nil {
		return s.resultFromStored(*applied.Replay, input.subject)
	}
	result := s.resultFromApplied(
		applied, resolvedAuthorID(input, item), processed, previousContent, classification.Risk, action, plan,
	)
	result.Images = moderationrepo.AuthorOriginalImageViews(input.imageViews)
	result.Content = rewriteImageKeys(result.Content, input.imageViews)
	if result.PendingContent != nil {
		value := rewriteImageKeys(*result.PendingContent, input.imageViews)
		result.PendingContent = &value
	}
	return result, nil
}

func resolvedAuthorID(input writeInput, item moderationrepo.ItemStateRecord) uint64 {
	if input.isEdit {
		return item.AuthorID
	}
	if input.authorID != 0 {
		return input.authorID
	}
	return input.actorID
}

func (s *applicationService) validateWrite(input writeInput) error {
	if input.actorID == 0 || s.repo == nil || s.classifier == nil || input.content == "" {
		return ErrInvalidRequest
	}
	if input.idempotencyKey == "" || len(input.idempotencyKey) > maxIdempotencyKeyChars ||
		strings.TrimSpace(input.idempotencyKey) != input.idempotencyKey ||
		strings.IndexFunc(input.idempotencyKey, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: invalid idempotency key", ErrInvalidRequest)
	}
	if input.isEdit == (input.subject.ID == 0) {
		return fmt.Errorf("%w: subject ID does not match operation", ErrInvalidRequest)
	}
	if input.isEdit && input.authorID != 0 {
		return fmt.Errorf("%w: edit cannot override author", ErrInvalidRequest)
	}
	if !input.isEdit && input.authorID != 0 && input.authorID != input.actorID && !input.isAdmin {
		return fmt.Errorf("%w: author override requires admin", ErrInvalidRequest)
	}
	if input.momentOptions != nil {
		if input.subject.Type != SubjectMoment || input.momentOptions.Status > 1 || input.momentOptions.CommentStatus > 1 {
			return fmt.Errorf("%w: invalid moment options", ErrInvalidRequest)
		}
	}
	if input.isEdit && validSubjectType(input.subject.Type) {
		return nil
	}
	return validateSubjectRef(input.subject)
}

func cloneMomentOptions(options *moderationrepo.MomentOptions) *moderationrepo.MomentOptions {
	if options == nil {
		return nil
	}
	result := *options
	return &result
}

func validSubjectType(subjectType SubjectType) bool {
	switch subjectType {
	case SubjectMoment, SubjectArticleComment, SubjectMomentComment, SubjectGuestbook,
		SubjectArticleCommentReply, SubjectMomentCommentReply, SubjectGuestbookReply:
		return true
	default:
		return false
	}
}

func validateSubjectRef(ref SubjectRef) error {
	switch ref.Type {
	case SubjectMoment:
		if ref.RootID == 0 && ref.ParentID == nil {
			return nil
		}
	case SubjectArticleComment, SubjectMomentComment, SubjectGuestbook:
		if ref.RootID != 0 && ref.ParentID == nil {
			return nil
		}
	case SubjectArticleCommentReply, SubjectMomentCommentReply, SubjectGuestbookReply:
		if ref.RootID != 0 && ref.ParentID != nil {
			return nil
		}
	}
	return fmt.Errorf("%w: invalid subject relation", ErrInvalidRequest)
}

func (s *applicationService) contentLimit(subjectType SubjectType) int {
	switch subjectType {
	case SubjectMoment:
		return s.cfg.Content.MomentMaxChars
	case SubjectArticleComment, SubjectMomentComment:
		return s.cfg.Content.CommentMaxChars
	case SubjectGuestbook:
		return s.cfg.Content.GuestbookMaxChars
	default:
		return s.cfg.Content.ReplyMaxChars
	}
}

func policyInput(
	input writeInput,
	policyContext moderationrepo.PolicyContext,
	classification Classification,
	hasLink bool,
	hasUnapprovedImage bool,
	cfg config.ModerationConfig,
) PolicyInput {
	return PolicyInput{
		IsAdmin: input.isAdmin, Trust: TrustLevel(policyContext.TrustLevel),
		Sanction:   SanctionState(policyContext.SanctionState),
		Publishing: PublishingMode(policyContext.PublishingMode),
		Risk:       classification.Risk, HasUnapprovedImage: hasUnapprovedImage,
		HasExternalLinkOrAd: hasLink, Policy: cfg.Policy,
	}
}

func revisionImageDrafts(images []moderationmedia.PreparedImage) []moderationrepo.RevisionImageDraft {
	result := make([]moderationrepo.RevisionImageDraft, 0, len(images))
	for index, image := range images {
		result = append(result, moderationrepo.RevisionImageDraft{
			ImageFingerprint: moderationrepo.ImageFingerprint{
				SHA256: image.SHA256, MD5: image.MD5, Size: image.Size,
			},
			Seq: uint(index + 1), ObjectKey: image.ObjectKey, MediaType: image.MediaType, IsGIF: image.IsGIF,
		})
	}
	return result
}

func preparedImageViews(images []moderationmedia.PreparedImage) []moderationrepo.ImageView {
	result := make([]moderationrepo.ImageView, 0, len(images))
	for index, image := range images {
		displayKey := image.ObjectKey
		if !image.Approved {
			displayKey = image.PreviewObjectKey
		}
		result = append(result, moderationrepo.ImageView{
			Seq: uint(index + 1), SourceObjectKey: image.ObjectKey, DisplayObjectKey: displayKey,
			Approved: image.Approved, IsGIF: image.IsGIF,
		})
	}
	return result
}

func applyImageReplacements(content string, replacements map[string]string) string {
	for source, target := range replacements {
		if source != "" && target != "" && source != target {
			content = strings.ReplaceAll(content, source, target)
		}
	}
	return content
}

func publishingBlocked(isAdmin bool, policy moderationrepo.PolicyContext) bool {
	if isAdmin {
		return false
	}
	return policy.SanctionState == moderationrepo.SanctionMuted ||
		policy.SanctionState == moderationrepo.SanctionBanned ||
		policy.PublishingMode == moderationrepo.PublishingClosed
}

func chooseSubmitEvent(edit bool) Event {
	if edit {
		return EventResubmit
	}
	return EventSubmit
}

func (s *applicationService) recordRiskBlock(
	ctx context.Context,
	input writeInput,
	item moderationrepo.ItemStateRecord,
	classification Classification,
) (SubmitResult, error) {
	var itemID *uint64
	if item.ItemID != 0 {
		value := item.ItemID
		itemID = &value
	}
	now := s.now()
	_, err := s.repo.RecordBlockedAttempt(ctx, moderationrepo.BlockedAttempt{
		UserID: input.actorID, SubjectType: input.subject.Type, ItemID: itemID,
		IdempotencyKey: input.idempotencyKey, RulesetVersion: classification.RulesetVersion,
		RuleMatchIDs: classification.RuleMatchIDs, RuleMatchesTruncated: classification.RuleMatchesTruncated, CreatedAt: now,
		ProfileChange: &moderationrepo.ProfileChange{
			UserID: input.actorID, HighRiskDelta: 1,
			ViolationScoreDelta: int64(s.cfg.Governance.ViolationWeights.HighRiskBlocked),
			ResetCleanApproval:  true, LastViolationAt: &now, UpdatedAt: now,
		},
	})
	if errors.Is(err, moderationrepo.ErrIdempotencyDomainConflict) {
		return s.resolveRace(ctx, input)
	}
	if err != nil {
		s.logger.Error("记录高风险审核尝试失败",
			zap.Uint64("user_id", input.actorID),
			zap.String("content_type", string(input.subject.Type)),
			zap.Error(err),
		)
	} else if governanceConfigured(s.cfg.Governance) {
		if _, governanceErr := reconcileProfile(ctx, s.repo, input.actorID, s.cfg.Governance, now); governanceErr != nil {
			s.logger.Warn("刷新用户审核画像失败，将在下次访问时重试",
				zap.Uint64("user_id", input.actorID), zap.Error(governanceErr))
		}
	}
	message := s.cfg.Notices.HighRejected
	if input.isEdit {
		message += "，原内容不受影响。"
	}
	return SubmitResult{}, fmt.Errorf("%w: %s", ErrContentRiskRejected, message)
}

func (s *applicationService) resolveRace(ctx context.Context, input writeInput) (SubmitResult, error) {
	stored, err := s.repo.FindResultByIdempotencyKey(ctx, input.actorID, input.idempotencyKey)
	if err != nil {
		return SubmitResult{}, err
	}
	if stored == nil {
		return SubmitResult{}, moderationrepo.ErrIdempotencyDomainConflict
	}
	return s.resultFromStored(*stored, input.subject)
}
