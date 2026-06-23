package notification

import (
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
)

// eventTypeLabels 把事件类型映射为中文展示词，用于兜底分支的标题前缀。
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

// 根对象中文称谓：邮件正文中指代「你的文章/碎语/留言板」时使用。
const (
	rootLabelArticle   = "文章"
	rootLabelMoment    = "碎语"
	rootLabelGuestbook = "留言板"
)

// rootLabelFor 返回根对象类型的中文称谓，未知类型回退为「内容」。
func rootLabelFor(rootType string) string {
	switch rootType {
	case "article":
		return rootLabelArticle
	case "moment":
		return rootLabelMoment
	case "guestbook":
		return rootLabelGuestbook
	default:
		return "内容"
	}
}

// renderDigestHTML 渲染摘要邮件正文：按任务顺序列出每条通知的具体场景与内容快照，
// 文章类根对象附上可点击跳转链接，底部提供返回站点的 Footer。
//
//   - rootLabels 按事件 ID 索引的根对象展示快照（文章标题/碎语摘要），缺失时回退为「ID xx」。
//   - siteURL 站点前缀，非空时用于文章跳转链接与 Footer 链接；为空时退化为纯文本。
func renderDigestHTML(tasks []model.NotificationEmailTask, events map[uint]model.NotificationEvent, rootLabels map[uint]string, siteURL string) string {
	siteURL = strings.TrimRight(siteURL, "/")
	var b strings.Builder
	b.WriteString(`<div><h2>你有新的互动通知</h2><ul>`)
	for _, task := range tasks {
		event := events[task.EventID]
		eventType := eventTypeOf(task, event)
		summary := html.EscapeString(eventSummary(event))
		b.WriteString("<li>")
		b.WriteString(renderEventLine(eventType, event, rootLabels, siteURL, summary))
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
	b.WriteString(renderFooter(siteURL))
	b.WriteString("</div>")
	return b.String()
}

// renderEventLine 渲染单条通知正文，按事件类型与根对象类型组合具体场景文案。
func renderEventLine(eventType string, event model.NotificationEvent, rootLabels map[uint]string, siteURL, summary string) string {
	rootLabel := html.EscapeString(truncateForDisplay(rootLabels[event.ID]))
	switch eventType {
	case EventTypeCommentCreated:
		return renderCommentCreatedLine(event, rootLabel, siteURL, summary)
	case EventTypeReplyCreated:
		return renderReplyCreatedLine(event, rootLabel, siteURL, summary)
	case EventTypeGuestbookCreated:
		return "你的留言板收到了新留言：" + summary
	default:
		// 兜底：未知事件类型沿用旧版「类型：摘要」结构。
		return fmt.Sprintf("<strong>%s</strong>：%s", html.EscapeString(eventTypeLabel(eventType)), summary)
	}
}

// renderCommentCreatedLine 渲染「被评论」通知行：
// 文章根附带跳转链接，碎语/留言根以中文称谓标识；根快照缺失时回退为 ID。
func renderCommentCreatedLine(event model.NotificationEvent, rootLabel, siteURL, summary string) string {
	URL := articleURL(event.RootID, siteURL)
	switch event.RootType {
	case "article":
		if rootLabel == "" {
			rootLabel = "ID " + strconv.FormatUint(uint64(event.RootID), 10)
		}
		if URL != "" {
			return fmt.Sprintf(`你的文章「<a href="%s">%s</a>」收到了新评论：%s`, template.HTMLEscapeString(URL), rootLabel, summary)
		}
		return fmt.Sprintf(`你的文章「%s」收到了新评论：%s`, rootLabel, summary)
	case "moment":
		if rootLabel == "" {
			rootLabel = "ID " + strconv.FormatUint(uint64(event.RootID), 10)
		}
		return fmt.Sprintf(`你的碎语「%s」收到了新评论：%s`, rootLabel, summary)
	case "guestbook":
		return "你的留言板收到了新留言：" + summary
	default:
		return "你的" + rootLabelFor(event.RootType) + "收到了新评论：" + summary
	}
}

// renderReplyCreatedLine 渲染「被回复」通知行：始终以「回复」为语义主体，并用根快照补充上下文。
func renderReplyCreatedLine(event model.NotificationEvent, rootLabel, siteURL, summary string) string {
	URL := articleURL(event.RootID, siteURL)
	switch event.RootType {
	case "article":
		if rootLabel == "" {
			rootLabel = "ID " + strconv.FormatUint(uint64(event.RootID), 10)
		}
		if URL != "" {
			return fmt.Sprintf(`你在文章「<a href="%s">%s</a>」下的评论收到了新回复：%s`, template.HTMLEscapeString(URL), rootLabel, summary)
		}
		return fmt.Sprintf(`你在文章「%s」下的评论收到了新回复：%s`, rootLabel, summary)
	case "moment":
		if rootLabel == "" {
			rootLabel = "ID " + strconv.FormatUint(uint64(event.RootID), 10)
		}
		return fmt.Sprintf(`你在碎语「%s」下的评论收到了新回复：%s`, rootLabel, summary)
	case "guestbook":
		return "你的留言收到了新回复：" + summary
	default:
		return "你的评论收到了新回复：" + summary
	}
}

// articleURL 在站点前缀存在且根对象为文章时，拼出 /articles/{id} 的可跳转链接；
// 其余情况返回空串表示不渲染跳转。
func articleURL(rootID uint, siteURL string) string {
	if siteURL == "" || rootID == 0 {
		return ""
	}
	return siteURL + "/articles/" + strconv.FormatUint(uint64(rootID), 10)
}

// renderFooter 渲染底部「欢迎回到 YEVPT 查看详情」；站点前缀存在时整句作可点击链接。
func renderFooter(siteURL string) string {
	if siteURL == "" {
		return `<p>欢迎回到 YEVPT 查看详情。</p>`
	}
	return fmt.Sprintf(`<p><a href="%s">欢迎回到 YEVPT 查看详情</a></p>`, template.HTMLEscapeString(siteURL))
}

// truncateForDisplay 折叠展示快照中的换行与首尾空白，避免邮件正文中出现不可控排版。
// 长度截断由 Directory 在快照写入时完成，此处仅做空规整。
func truncateForDisplay(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
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
