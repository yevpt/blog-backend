package moderationemail_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
	moderationemailservice "github.com/vpt/blog-backend/internal/service/moderationemail"
)

func TestRenderEscapesSubmittedHTMLAndKeepsUnicodeExcerptValid(t *testing.T) {
	batch := reviewBatch(7, 2)
	long := strings.Repeat("中🙂", 80) + `<script>alert("x")</script>`
	tasks := []moderationemailrepo.PendingTask{
		reviewTask(1, model.ModerationContentMoment, `<img src=x onerror=alert(1)>`),
		reviewTask(2, model.ModerationContentArticleComment, long),
	}

	rendered, err := moderationemailservice.Render(batch, tasks, " https://blog.example.com/base/ ")

	require.NoError(t, err)
	assert.Equal(t, "待审核内容提醒（2 条）", rendered.Subject)
	assert.Contains(t, rendered.HTML, "共 2 条待审核内容")
	assert.Contains(t, rendered.HTML, "碎语")
	assert.Contains(t, rendered.HTML, "文章评论")
	assert.Contains(t, rendered.HTML, "作者 #301")
	assert.Contains(t, rendered.HTML, "https://blog.example.com/base/admin/moderation")
	assert.NotContains(t, rendered.HTML, `<img src=x`)
	assert.NotContains(t, rendered.HTML, "<script>")
	assert.Contains(t, rendered.HTML, "&lt;img src=x")
	assert.True(t, utf8.ValidString(rendered.HTML))
}

func TestRenderLimitsRowsAndShowsOverflowAgainstTotalItemCount(t *testing.T) {
	batch := reviewBatch(8, 53)
	tasks := make([]moderationemailrepo.PendingTask, 0, 53)
	for i := 0; i < 53; i++ {
		tasks = append(tasks, reviewTask(uint64(i+1), model.ModerationContentGuestbookReply, "留言回复"))
	}

	rendered, err := moderationemailservice.Render(batch, tasks, "")

	require.NoError(t, err)
	assert.Equal(t, "待审核内容提醒（53 条）", rendered.Subject)
	assert.Contains(t, rendered.HTML, "共 53 条待审核内容")
	assert.Contains(t, rendered.HTML, "另有 3 条未展示")
	assert.NotContains(t, rendered.HTML, `<a href=`)
	assert.Equal(t, 50, strings.Count(rendered.HTML, "<tr>"))
}

func reviewBatch(id uint64, itemCount int) model.ModerationReviewEmailBatch {
	return model.ModerationReviewEmailBatch{
		ID:        id,
		ToEmail:   "owner@example.com",
		ItemCount: itemCount,
		Subject:   "legacy subject should be replaced",
	}
}

func reviewTask(id uint64, contentType string, content string) moderationemailrepo.PendingTask {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	return moderationemailrepo.PendingTask{
		ID:               id,
		RevisionID:       id + 100,
		ItemID:           id + 200,
		ContentType:      contentType,
		AuthorID:         id + 300,
		SubmittedContent: content,
		AvailableAt:      now,
		CreatedAt:        now,
	}
}
