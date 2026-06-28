package moderation

import (
	"strings"
	"time"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/pkg/config"
)

func buildReviewTransition(
	record moderationrepo.ReviewRecord,
	cmd ReviewCommand,
	event Event,
	corrected *string,
	plan TransitionPlan,
	now time.Time,
	cfg config.ModerationConfig,
) moderationrepo.ApplyTransitionCommand {
	reason := strings.TrimSpace(cmd.Reason)
	reviewerID := cmd.ReviewerID
	review := plan.ReviewRevision
	persisted := moderationrepo.ApplyTransitionCommand{
		Subject: record.Subject, AuthorID: record.AuthorID,
		ExpectedLockVersion: record.LockVersion, ExpectedPendingID: uint64Pointer(record.RevisionID),
		Next: itemState(plan.Item), Materialize: mapRevisionPointer(plan.MaterializeRevision),
		MomentOptions: cloneMomentOptions(record.MomentOptions),
		Review: &moderationrepo.RevisionReview{
			RevisionID: review.RevisionID, Status: moderationrepo.ReviewStatus(review.Status),
			Decision: string(review.Decision), Reason: optionalReviewReason(reason),
			ReviewerID: &reviewerID, ReviewedAt: now, PublishedContent: cloneString(corrected),
		},
		Log: &moderationrepo.ActionLog{
			Revision: moderationrepo.ExistingRevision(record.RevisionID), ActorUserID: &reviewerID,
			SubjectUserID: uint64Pointer(record.AuthorID), Action: moderationrepo.Event(event),
			Reason: optionalReviewReason(reason), CreatedAt: now,
		},
		ProfileChange: reviewProfileChange(event, record.AuthorID, now, cfg),
		Notification:  reviewNotification(event, record, reason),
		SyncImages:    true,
	}
	return persisted
}

func reviewProfileChange(event Event, authorID uint64, now time.Time, cfg config.ModerationConfig) *moderationrepo.ProfileChange {
	change := &moderationrepo.ProfileChange{UserID: authorID, UpdatedAt: now}
	switch event {
	case EventApprove:
		change.CleanApprovalDelta = 1
	case EventCorrectAndApprove:
		change.CorrectedDelta = 1
		change.ViolationScoreDelta = int64(cfg.Governance.ViolationWeights.Corrected)
		change.ResetCleanApproval = true
		change.LastViolationAt = &now
	case EventReject:
		change.RejectedDelta = 1
		change.ViolationScoreDelta = int64(cfg.Governance.ViolationWeights.Rejected)
		change.ResetCleanApproval = true
		change.LastViolationAt = &now
	}
	return change
}

func reviewNotification(event Event, record moderationrepo.ReviewRecord, reason string) *moderationrepo.NotificationIntent {
	title := "内容审核通过"
	excerpt := "你的内容已通过审核。"
	decision := "approved"
	switch event {
	case EventCorrectAndApprove:
		title = "内容经管理员修正后已发布"
		excerpt = reason
		decision = "corrected"
	case EventReject:
		title = "内容审核未通过"
		excerpt = reason
		decision = "rejected"
	}
	return &moderationrepo.NotificationIntent{
		RecipientUserID: record.AuthorID, Title: title, ContentExcerpt: excerpt,
		ItemID: record.ItemID, RevisionID: record.RevisionID, Decision: decision,
	}
}

func appliedReviewItem(
	record moderationrepo.ReviewRecord,
	cmd ReviewCommand,
	corrected *string,
	plan TransitionPlan,
	lockVersion uint64,
	now time.Time,
) ReviewItem {
	record.State = itemState(plan.Item)
	record.LockVersion = lockVersion
	record.ReviewStatus = moderationrepo.ReviewStatus(plan.ReviewRevision.Status)
	decision := string(plan.ReviewRevision.Decision)
	record.DecisionType = &decision
	reason := strings.TrimSpace(cmd.Reason)
	record.DecisionReason = optionalReviewReason(reason)
	reviewerID := cmd.ReviewerID
	record.ReviewerID = &reviewerID
	record.ReviewedAt = &now
	if corrected != nil {
		record.PublishedContent = *corrected
	}
	return reviewItemFromRecord(record)
}

func reviewItemFromRecord(record moderationrepo.ReviewRecord) ReviewItem {
	return ReviewItem{
		ItemID: record.ItemID, Subject: record.Subject, AuthorID: record.AuthorID,
		LockVersion: record.LockVersion, LifecycleState: LifecycleState(record.State.LifecycleState),
		PublicState: PublicState(record.State.PublicState), RevisionID: record.RevisionID,
		RevisionVersion: record.RevisionVersion, SubmittedContent: record.SubmittedContent,
		PublishedContent: record.PublishedContent, RiskLevel: RiskLevel(record.RiskLevel),
		PolicyAction: PolicyAction(record.PolicyAction), ReviewStatus: ReviewStatus(record.ReviewStatus),
		MomentOptions: cloneMomentOptions(record.MomentOptions), DecisionType: cloneString(record.DecisionType),
		DecisionReason: cloneString(record.DecisionReason), ReviewerID: cloneUint64(record.ReviewerID),
		ReviewedAt: cloneTime(record.ReviewedAt), CreatedAt: record.CreatedAt,
		CanInteract: itemSnapshot(record.State).CanInteract(),
	}
}

func optionalReviewReason(reason string) *string {
	if reason == "" {
		return nil
	}
	return &reason
}
