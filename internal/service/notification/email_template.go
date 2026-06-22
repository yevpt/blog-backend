package notification

import (
	"fmt"
	"html"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
)

// eventTypeLabels 把事件类型映射为中文展示词。
var eventTypeLabels = map[string]string{
	EventTypeCommentCreated:   "评论",
	EventTypeReplyCreated:     "回复",
	EventTypeArticleLiked:     "文章点赞",
	EventTypeMomentLiked:      "碎语点赞",
	EventTypeGuestbookCreated: "留言",
	EventTypeGuestbookLiked:   "留言点赞",
	EventTypeSystemNotice:     "系统通知",
	EventTypeLegacyNotice:     "通知",
}

// renderDigestHTML 渲染摘要邮件正文，按任务顺序列出每条通知的类型与内容快照。
// events 为任务对应事件的快照映射；缺失的事件以任务自带的类型兜底。
func renderDigestHTML(tasks []model.NotificationEmailTask, events map[uint]model.NotificationEvent) string {
	var b strings.Builder
	b.WriteString(`<div><h2>你有新的互动通知</h2><ul>`)
	for _, task := range tasks {
		event := events[task.EventID]
		label := eventTypeLabel(eventTypeOf(task, event))
		summary := html.EscapeString(eventSummary(event))
		b.WriteString(fmt.Sprintf("<li><strong>%s</strong>：%s</li>", html.EscapeString(label), summary))
	}
	b.WriteString(`</ul><p>登录博客查看详情。</p></div>`)
	return b.String()
}

// eventTypeOf 取事件类型，事件快照缺失时回退到任务上的类型快照。
func eventTypeOf(task model.NotificationEmailTask, event model.NotificationEvent) string {
	if event.Type != "" {
		return event.Type
	}
	return task.EventType
}

// eventTypeLabel 返回事件类型的中文展示词，未知类型回退为「通知」。
func eventTypeLabel(eventType string) string {
	if label, ok := eventTypeLabels[eventType]; ok {
		return label
	}
	return "通知"
}

// eventSummary 取事件的展示文案：优先内容摘要，其次标题，均空时给出占位语。
func eventSummary(event model.NotificationEvent) string {
	if event.ContentExcerpt != "" {
		return event.ContentExcerpt
	}
	if event.Title != "" {
		return event.Title
	}
	return "你有一条新通知"
}
