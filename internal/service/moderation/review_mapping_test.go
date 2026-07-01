package moderation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/pkg/config"
)

func TestReviewApprovedContentExcerptPrefersPublishedContent(t *testing.T) {
	record := moderationrepo.ReviewRecord{
		PublishedContent: "  待审正文  ",
		SubmittedContent: "用户原文",
	}
	assert.Equal(t, "待审正文", reviewApprovedContentExcerpt(record))
}

func TestReviewApprovedContentExcerptFallsBackToSubmittedContent(t *testing.T) {
	record := moderationrepo.ReviewRecord{
		PublishedContent: "   ",
		SubmittedContent: "用户原文",
	}
	assert.Equal(t, "用户原文", reviewApprovedContentExcerpt(record))
}

func TestReviewNotificationApprovedUsesContentExcerpt(t *testing.T) {
	record := moderationrepo.ReviewRecord{
		ItemID: 10, RevisionID: 20, AuthorID: 42,
		PublishedContent: "写得真好",
	}
	intent := reviewNotification(EventApprove, record, "", moderationrepo.ReviewNotificationContext{
		ContentType: moderationrepo.SubjectMoment,
	})
	require.NotNil(t, intent)
	assert.Equal(t, "写得真好", intent.ContentExcerpt)
	assert.Equal(t, "approved", intent.Decision)
}

func TestInteractionNotificationRepositoryContract(t *testing.T) {
	commentID := uint64(7)
	rootSnapshot := &moderationrepo.NotificationSnapshot{Type: "article", ID: 5}
	quoteSnapshot := &moderationrepo.NotificationSnapshot{Type: "comment", ID: commentID}
	intent := &moderationrepo.InteractionNotificationIntent{
		Type: "comment_created", ActorUserID: 42, RecipientUserID: 99,
		SourceType: "comment", SourceID: 0, RootType: "article", RootID: 5,
		ContentExcerpt: "首次公开评论", CommentID: &commentID,
		RootSnapshot: rootSnapshot, QuoteSnapshot: quoteSnapshot,
	}
	context := moderationrepo.ReviewNotificationContext{
		ContentType: moderationrepo.SubjectArticleComment, InteractionRecipientUserID: 99,
	}
	command := moderationrepo.ApplyTransitionCommand{InteractionNotification: intent}

	assert.Equal(t, uint64(99), context.InteractionRecipientUserID)
	require.Same(t, intent, command.InteractionNotification)
	assert.Equal(t, "comment_created", command.InteractionNotification.Type)
	assert.Equal(t, uint64(42), command.InteractionNotification.ActorUserID)
	assert.Equal(t, uint64(99), command.InteractionNotification.RecipientUserID)
	assert.Equal(t, "comment", command.InteractionNotification.SourceType)
	assert.Zero(t, command.InteractionNotification.SourceID)
	assert.Equal(t, "article", command.InteractionNotification.RootType)
	assert.Equal(t, uint64(5), command.InteractionNotification.RootID)
	assert.Equal(t, "首次公开评论", command.InteractionNotification.ContentExcerpt)
	assert.Same(t, &commentID, command.InteractionNotification.CommentID)
	assert.Same(t, rootSnapshot, command.InteractionNotification.RootSnapshot)
	assert.Same(t, quoteSnapshot, command.InteractionNotification.QuoteSnapshot)
}

func TestBuildReviewTransitionFirstPublicationInteraction(t *testing.T) {
	commentID := uint64(201)
	tests := []struct {
		name       string
		subject    moderationrepo.SubjectRef
		context    moderationrepo.ReviewNotificationContext
		eventType  string
		sourceType string
		rootType   string
		rootID     uint64
		commentID  *uint64
	}{
		{
			name: "article comment", subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 7, RootID: 101},
			context: moderationrepo.ReviewNotificationContext{
				InteractionRecipientUserID: 91,
				RootSnapshot:               &moderationrepo.NotificationSnapshot{Type: "article", ID: 101},
			},
			eventType: "comment_created", sourceType: "comment", rootType: "article", rootID: 101,
		},
		{
			name: "moment comment", subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectMomentComment, ID: 8, RootID: 102},
			context: moderationrepo.ReviewNotificationContext{
				InteractionRecipientUserID: 92,
				RootSnapshot:               &moderationrepo.NotificationSnapshot{Type: "moment", ID: 102},
			},
			eventType: "comment_created", sourceType: "comment", rootType: "moment", rootID: 102,
		},
		{
			name: "guestbook", subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectGuestbook, ID: 9, RootID: 93},
			context: moderationrepo.ReviewNotificationContext{
				InteractionRecipientUserID: 93,
				RootSnapshot:               &moderationrepo.NotificationSnapshot{Type: "guestbook", ID: 9},
			},
			eventType: "guestbook_created", sourceType: "guestbook", rootType: "guestbook", rootID: 9,
		},
		{
			name: "article reply", subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleCommentReply, ID: 10, RootID: commentID},
			context: moderationrepo.ReviewNotificationContext{
				InteractionRecipientUserID: 94, CommentID: &commentID,
				RootSnapshot: &moderationrepo.NotificationSnapshot{Type: "article", ID: 101},
			},
			eventType: "reply_created", sourceType: "reply", rootType: "article", rootID: 101, commentID: &commentID,
		},
		{
			name: "moment reply", subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectMomentCommentReply, ID: 11, RootID: 202},
			context: moderationrepo.ReviewNotificationContext{
				InteractionRecipientUserID: 95, CommentID: uint64Pointer(202),
				RootSnapshot: &moderationrepo.NotificationSnapshot{Type: "moment", ID: 102},
			},
			eventType: "reply_created", sourceType: "reply", rootType: "moment", rootID: 102, commentID: uint64Pointer(202),
		},
		{
			name: "guestbook reply", subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectGuestbookReply, ID: 12, RootID: 203},
			context: moderationrepo.ReviewNotificationContext{
				InteractionRecipientUserID: 96, CommentID: uint64Pointer(203),
				RootSnapshot: &moderationrepo.NotificationSnapshot{Type: "guestbook", ID: 9},
			},
			eventType: "reply_created", sourceType: "reply", rootType: "guestbook", rootID: 9, commentID: uint64Pointer(203),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, scenario := range []struct {
				name          string
				event         Event
				priorApproved bool
				wantIntent    bool
			}{
				{name: "approve", event: EventApprove, wantIntent: true},
				{name: "correct and approve", event: EventCorrectAndApprove, wantIntent: true},
				{name: "reject", event: EventReject},
				{name: "approved edit", event: EventApprove, priorApproved: true},
				{name: "approved corrected edit", event: EventCorrectAndApprove, priorApproved: true},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					record := moderationrepo.ReviewRecord{
						ItemID: 1, RevisionID: 2, AuthorID: 42, Subject: tt.subject,
						PublishedContent: "首次公开正文",
					}
					if scenario.priorApproved {
						record.State.Approved = moderationrepo.ExistingRevision(1)
					}
					correctedContent := "管理员修正正文"
					var corrected *string
					if scenario.event == EventCorrectAndApprove {
						corrected = &correctedContent
					}
					cmd := buildReviewTransition(
						record,
						ReviewCommand{ReviewerID: 1},
						scenario.event,
						corrected,
						interactionReviewPlan(scenario.event),
						time.Unix(1, 0),
						config.ModerationConfig{},
						tt.context,
					)
					if !scenario.wantIntent {
						assert.Nil(t, cmd.InteractionNotification)
						return
					}
					require.NotNil(t, cmd.InteractionNotification)
					intent := cmd.InteractionNotification
					assert.Equal(t, tt.eventType, intent.Type)
					assert.Equal(t, record.AuthorID, intent.ActorUserID)
					assert.Equal(t, tt.context.InteractionRecipientUserID, intent.RecipientUserID)
					assert.Equal(t, tt.sourceType, intent.SourceType)
					assert.Zero(t, intent.SourceID)
					assert.Equal(t, tt.rootType, intent.RootType)
					assert.Equal(t, tt.rootID, intent.RootID)
					assert.Equal(t, tt.commentID, intent.CommentID)
					assert.Same(t, tt.context.RootSnapshot, intent.RootSnapshot)
					assert.Same(t, tt.context.QuoteSnapshot, intent.QuoteSnapshot)
					wantContent := record.PublishedContent
					if scenario.event == EventCorrectAndApprove {
						wantContent = correctedContent
					}
					assert.Equal(t, wantContent, intent.ContentExcerpt)
				})
			}
		})
	}
}

func interactionReviewPlan(event Event) TransitionPlan {
	decision := DecisionApproved
	status := ReviewApproved
	if event == EventCorrectAndApprove {
		decision = DecisionCorrected
	}
	if event == EventReject {
		decision = DecisionRejected
		status = ReviewRejected
	}
	return TransitionPlan{ReviewRevision: &ReviewRevisionIntent{RevisionID: 2, Status: status, Decision: decision}}
}
