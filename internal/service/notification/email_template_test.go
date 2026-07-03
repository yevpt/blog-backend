package notification

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vpt/blog-backend/internal/model"
)

func TestBadgeFor_ReturnsDistinctLabelsForEligibleEventTypes(t *testing.T) {
	cases := []struct {
		eventType string
		wantLabel string
	}{
		{EventTypeCommentCreated, "评论"},
		{EventTypeReplyCreated, "回复"},
		{EventTypeGuestbookCreated, "留言"},
	}
	for _, c := range cases {
		badge := badgeFor(c.eventType)
		assert.Equal(t, c.wantLabel, badge.Label)
		assert.NotEmpty(t, badge.BgColor)
		assert.NotEmpty(t, badge.FgColor)
	}
}

func TestBadgeFor_FallsBackToNeutralColorForUnknownType(t *testing.T) {
	badge := badgeFor(EventTypeSystemNotice)

	assert.Equal(t, "系统通知", badge.Label)
	assert.Equal(t, "#F1EFE8", badge.BgColor)
}

func TestRenderDigestHTML_WrapsCardsWithBrandAndCTA(t *testing.T) {
	tasks := []model.NotificationEmailTask{
		{Base: model.Base{ID: 1}, EventID: 100},
	}
	events := map[uint]model.NotificationEvent{
		100: {
			Base:           model.Base{ID: 100},
			Type:           EventTypeCommentCreated,
			RootType:       "article",
			RootID:         5,
			ContentExcerpt: "写得很好",
		},
	}
	rootLabels := map[uint]string{100: "如何设计邮件模板"}

	html := renderDigestHTML(tasks, events, rootLabels, "", "https://www.yevpt.com")

	assert.Contains(t, html, "你有 1 条新的互动通知")
	assert.Contains(t, html, "评论")
	assert.Contains(t, html, "写得很好")
	assert.Contains(t, html, "查看全部通知")
	assert.Contains(t, html, `href="https://www.yevpt.com"`)
	assert.Contains(t, html, ">YEVPT<")
	assert.Contains(t, html, `href="https://www.yevpt.com/articles/5"`)
}

func TestRenderDigestHTML_FallsBackToDefaultBrandWhenEmpty(t *testing.T) {
	html := renderDigestHTML(nil, nil, nil, "", "")

	assert.Contains(t, html, ">YEVPT<")
	assert.Contains(t, html, `href="https://www.yevpt.com"`)
}
