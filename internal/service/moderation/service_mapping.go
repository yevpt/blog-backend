package moderation

import (
	"fmt"
	"time"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
)

func (s *applicationService) transitionCommand(
	input writeInput,
	item moderationrepo.ItemStateRecord,
	processed ProcessedContent,
	classification Classification,
	action PolicyAction,
	plan TransitionPlan,
	interactionIntent *moderationrepo.InteractionNotificationIntent,
) moderationrepo.ApplyTransitionCommand {
	now := plan.AppendLog.CreatedAt
	authorID := input.actorID
	if input.isEdit {
		authorID = item.AuthorID
	} else if input.authorID != 0 {
		authorID = input.authorID
	}
	reviewStatus := moderationrepo.ReviewPending
	var decisionType *string
	var reviewerID *uint64
	var reviewedAt *time.Time
	if action == ActionAutoApprove {
		reviewStatus = moderationrepo.ReviewApproved
		decision := string(DecisionApproved)
		decisionType = &decision
		if input.isAdmin {
			reviewer := input.actorID
			reviewerID = &reviewer
		}
		reviewedAt = &now
	}
	command := moderationrepo.ApplyTransitionCommand{
		Subject: input.subject, AuthorID: authorID,
		ExpectedLockVersion: item.LockVersion, ExpectedPendingID: existingID(item.State.Pending),
		Next: itemState(plan.Item), Revision: &moderationrepo.RevisionDraft{
			SubmitterID: input.actorID, IdempotencyKey: input.idempotencyKey,
			SubmittedContent: input.content, PublishedContent: processed.Published,
			RiskLevel:    moderationrepo.RiskLevel(classification.Risk),
			PolicyAction: moderationrepo.PolicyAction(action), ReviewStatus: reviewStatus,
			RulesetVersion: classification.RulesetVersion, RuleMatchIDs: classification.RuleMatchIDs,
			RuleMatchesTruncated: classification.RuleMatchesTruncated,
			DecisionType:         decisionType, ReviewerID: reviewerID, ReviewedAt: reviewedAt,
			MomentOptions: cloneMomentOptions(input.momentOptions),
			Images:        input.images,
		},
		SupersedeRevisionID: plan.SupersedeRevision,
		Materialize:         mapRevisionPointer(plan.MaterializeRevision),
		MomentOptions:       cloneMomentOptions(input.momentOptions),
		CreateSubject:       !input.isEdit,
		SyncImages:          true,
		Log: &moderationrepo.ActionLog{
			Revision: moderationrepo.NewRevision(), ActorUserID: &input.actorID,
			SubjectUserID: &authorID, Action: moderationrepo.Event(plan.AppendLog.Event),
			Reason: plan.AppendLog.Reason, CreatedAt: plan.AppendLog.CreatedAt,
		},
		InteractionNotification: interactionIntent,
	}
	if plan.Item.PendingRevisionID != nil && *plan.Item.PendingRevisionID == newRevisionSentinel {
		command.ReviewEmailTask = &moderationrepo.ReviewEmailTaskIntent{
			AvailableAt: now.Add(time.Duration(s.cfg.ReviewEmail.AggregationWindowSeconds) * time.Second),
		}
	}
	return command
}

func itemSnapshot(state moderationrepo.ItemState) ItemSnapshot {
	return ItemSnapshot{
		LifecycleState:         LifecycleState(state.LifecycleState),
		PublicState:            PublicState(state.PublicState),
		MaterializedRevisionID: existingID(state.Materialized),
		ApprovedRevisionID:     existingID(state.Approved),
		PendingRevisionID:      existingID(state.Pending),
		StateBeforeEmergency:   mapEmergencyState(state.StateBeforeEmergency),
		EmergencyHiddenReason:  cloneString(state.EmergencyReason),
		EmergencyHiddenAt:      cloneTime(state.EmergencyHiddenAt),
		DeletedAt:              cloneTime(state.DeletedAt),
	}
}

func itemState(snapshot ItemSnapshot) moderationrepo.ItemState {
	return moderationrepo.ItemState{
		LifecycleState:       moderationrepo.LifecycleState(snapshot.LifecycleState),
		PublicState:          moderationrepo.PublicState(snapshot.PublicState),
		Materialized:         mapRevisionPointer(snapshot.MaterializedRevisionID),
		Approved:             mapRevisionPointer(snapshot.ApprovedRevisionID),
		Pending:              mapRevisionPointer(snapshot.PendingRevisionID),
		StateBeforeEmergency: mapRepoEmergencyState(snapshot.StateBeforeEmergency),
		EmergencyReason:      cloneString(snapshot.EmergencyHiddenReason),
		EmergencyHiddenAt:    cloneTime(snapshot.EmergencyHiddenAt),
		DeletedAt:            cloneTime(snapshot.DeletedAt),
	}
}

func mapRevisionPointer(id *uint64) moderationrepo.RevisionRef {
	if id == nil {
		return moderationrepo.RevisionRef{}
	}
	if *id == newRevisionSentinel {
		return moderationrepo.NewRevision()
	}
	return moderationrepo.ExistingRevision(*id)
}

func existingID(ref moderationrepo.RevisionRef) *uint64 {
	if ref.IsNew || ref.ID == 0 {
		return nil
	}
	value := ref.ID
	return &value
}

func mapEmergencyState(state *moderationrepo.PublicState) *PublicState {
	if state == nil {
		return nil
	}
	value := PublicState(*state)
	return &value
}

func mapRepoEmergencyState(state *PublicState) *moderationrepo.PublicState {
	if state == nil {
		return nil
	}
	value := moderationrepo.PublicState(*state)
	return &value
}

func (s *applicationService) resultFromApplied(
	applied moderationrepo.AppliedTransition,
	authorID uint64,
	processed ProcessedContent,
	previousContent string,
	risk RiskLevel,
	action PolicyAction,
	plan TransitionPlan,
) SubmitResult {
	result := SubmitResult{
		Subject: applied.Subject, AuthorID: authorID, ItemID: applied.ItemID, RevisionID: applied.RevisionID,
		RevisionVersion: applied.RevisionVersion, LockVersion: applied.LockVersion,
		RiskLevel: risk, Action: action, PublicState: plan.Item.PublicState,
		HasPendingRevision: plan.Item.PendingRevisionID != nil,
		CanInteract:        plan.Item.CanInteract(),
	}
	if action == ActionPreReview {
		result.Message = s.cfg.Notices.ReviewRequired
		result.ReviewStatus = ReviewPending
		result.Content = previousContent
		result.PendingContent = cloneString(&processed.Published)
		return result
	}
	result.Content = processed.Published
	if action == ActionPostReview {
		result.Message = s.cfg.Notices.LowSubmitted
		result.ReviewStatus = ReviewPending
		result.PendingContent = cloneString(&processed.Published)
		return result
	}
	result.Message = s.cfg.Notices.Approved
	result.ReviewStatus = ReviewApproved
	return result
}

func (s *applicationService) resultFromStored(stored moderationrepo.StoredResult, requested SubjectRef) (SubmitResult, error) {
	if stored.Kind == moderationrepo.ResultBlocked {
		message := s.cfg.Notices.HighRejected
		if stored.ItemID != 0 {
			message += "，原内容不受影响。"
		}
		return SubmitResult{}, fmt.Errorf("%w: %s", ErrContentRiskRejected, message)
	}
	if stored.Subject.Type != requested.Type || (requested.ID != 0 && stored.Subject.ID != requested.ID) {
		return SubmitResult{}, moderationrepo.ErrIdempotencyDomainConflict
	}
	if requested.RootID != 0 {
		stored.Subject.RootID = requested.RootID
	}
	if requested.ParentID != nil {
		stored.Subject.ParentID = cloneUint64(requested.ParentID)
	}
	action := PolicyAction(stored.PolicyAction)
	result := SubmitResult{
		Subject: stored.Subject, AuthorID: stored.AuthorID, ItemID: stored.ItemID, RevisionID: stored.RevisionID,
		RevisionVersion: stored.RevisionVersion, LockVersion: stored.LockVersion,
		RiskLevel: RiskLevel(stored.RiskLevel), Action: action,
		PublicState: PublicState(stored.PublicState), ReviewStatus: ReviewStatus(stored.ReviewStatus),
		HasPendingRevision: stored.ReviewStatus == moderationrepo.ReviewPending,
	}
	if action == ActionPreReview {
		result.Content = stored.VisibleContent
	} else {
		result.Content = stored.Content
	}
	if stored.ReviewStatus == moderationrepo.ReviewPending {
		result.PendingContent = cloneString(&stored.Content)
	}
	switch action {
	case ActionPostReview:
		result.Message = s.cfg.Notices.LowSubmitted
	case ActionPreReview:
		result.Message = s.cfg.Notices.ReviewRequired
	case ActionAutoApprove:
		result.Message = s.cfg.Notices.Approved
	default:
		return SubmitResult{}, fmt.Errorf("%w: unsupported stored action", ErrInvalidTransition)
	}
	result.CanInteract = stored.ReviewStatus == moderationrepo.ReviewApproved &&
		stored.PublicState == moderationrepo.PublicVisible
	return result, nil
}
