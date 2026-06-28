package moderation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/zap"
)

const newRevisionSentinel = ^uint64(0)

type writeInput struct {
	actorID        uint64
	isAdmin        bool
	subject        SubjectRef
	content        string
	imageKeys      []string
	idempotencyKey string
	isEdit         bool
}

func (s *applicationService) Submit(ctx context.Context, cmd SubmitCommand) (SubmitResult, error) {
	return s.write(ctx, writeInput{
		actorID: cmd.ActorID, isAdmin: cmd.IsAdmin, subject: cmd.Subject,
		content: cmd.Content, imageKeys: cmd.ImageKeys, idempotencyKey: cmd.IdempotencyKey,
	})
}

func (s *applicationService) Edit(ctx context.Context, cmd EditCommand) (SubmitResult, error) {
	return s.write(ctx, writeInput{
		actorID: cmd.ActorID, isAdmin: cmd.IsAdmin, subject: cmd.Subject,
		content: cmd.Content, imageKeys: cmd.ImageKeys, idempotencyKey: cmd.IdempotencyKey,
		isEdit: true,
	})
}

func (s *applicationService) write(ctx context.Context, input writeInput) (SubmitResult, error) {
	if err := s.validateWrite(input); err != nil {
		return SubmitResult{}, err
	}
	if len(input.imageKeys) > 0 {
		return SubmitResult{}, ErrImageReviewUnavailable
	}

	stored, err := s.repo.FindResultByIdempotencyKey(ctx, input.actorID, input.idempotencyKey)
	if err != nil {
		return SubmitResult{}, err
	}
	if stored != nil {
		return s.resultFromStored(*stored, input.subject)
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
	action, err := s.decider.Decide(policyInput(input, policyContext, classification, len(processed.Links) > 0, s.cfg))
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
	return s.resultFromApplied(applied, processed, previousContent, classification.Risk, action, plan), nil
}

func (s *applicationService) validateWrite(input writeInput) error {
	if input.actorID == 0 || s.repo == nil || s.classifier == nil || input.content == "" {
		return ErrInvalidRequest
	}
	if input.idempotencyKey == "" || len(input.idempotencyKey) > 120 ||
		strings.TrimSpace(input.idempotencyKey) != input.idempotencyKey ||
		strings.IndexFunc(input.idempotencyKey, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: invalid idempotency key", ErrInvalidRequest)
	}
	if input.isEdit == (input.subject.ID == 0) {
		return fmt.Errorf("%w: subject ID does not match operation", ErrInvalidRequest)
	}
	return validateSubjectRef(input.subject)
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
	cfg config.ModerationConfig,
) PolicyInput {
	return PolicyInput{
		IsAdmin: input.isAdmin, Trust: TrustLevel(policyContext.TrustLevel),
		Sanction:   SanctionState(policyContext.SanctionState),
		Publishing: PublishingMode(policyContext.PublishingMode),
		Risk:       classification.Risk, HasExternalLinkOrAd: hasLink, Policy: cfg.Policy,
	}
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
	_, err := s.repo.RecordBlockedAttempt(ctx, moderationrepo.BlockedAttempt{
		UserID: input.actorID, SubjectType: input.subject.Type, ItemID: itemID,
		IdempotencyKey: input.idempotencyKey, RulesetVersion: classification.RulesetVersion,
		RuleMatchIDs: classification.RuleMatchIDs, CreatedAt: s.now(),
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
