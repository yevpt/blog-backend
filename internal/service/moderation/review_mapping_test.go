package moderation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
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
		SourceType: "article_comment", SourceID: 0, RootType: "article", RootID: 5,
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
	assert.Equal(t, "article_comment", command.InteractionNotification.SourceType)
	assert.Zero(t, command.InteractionNotification.SourceID)
	assert.Equal(t, "article", command.InteractionNotification.RootType)
	assert.Equal(t, uint64(5), command.InteractionNotification.RootID)
	assert.Equal(t, "首次公开评论", command.InteractionNotification.ContentExcerpt)
	assert.Same(t, &commentID, command.InteractionNotification.CommentID)
	assert.Same(t, rootSnapshot, command.InteractionNotification.RootSnapshot)
	assert.Same(t, quoteSnapshot, command.InteractionNotification.QuoteSnapshot)
}
