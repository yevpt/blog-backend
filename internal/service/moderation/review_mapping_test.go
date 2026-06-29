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
