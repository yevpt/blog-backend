package notification

import (
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/email/layout"
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

// eventBadge 是通知卡片左上角类型徽章的展示文案与配色。
type eventBadge struct {
	Label   string
	BgColor string
	FgColor string
}

// badgeFor 返回事件类型对应的徽章样式。邮件聚合队列当前只允许评论/回复/留言三种事件类型
// 进入邮件（见 dispatcher.go 的 emailEligibleEventTypes），其余类型统一使用中性配色兜底。
func badgeFor(eventType string) eventBadge {
	switch eventType {
	case EventTypeCommentCreated:
		return eventBadge{Label: "评论", BgColor: "#EEEDFE", FgColor: "#534AB7"}
	case EventTypeReplyCreated:
		return eventBadge{Label: "回复", BgColor: "#E1F5EE", FgColor: "#0F6E56"}
	case EventTypeGuestbookCreated:
		return eventBadge{Label: "留言", BgColor: "#FAECE7", FgColor: "#993C1D"}
	default:
		return eventBadge{Label: eventTypeLabel(eventType), BgColor: "#F1EFE8", FgColor: "#5F5E5A"}
	}
}

// renderDigestHTML 渲染摘要邮件正文：按任务顺序把每条通知渲染成带类型徽章的卡片，
// 文章类根对象附上可点击跳转链接；外层壳体（品牌 Logo、CTA 按钮、版权）由 layout.Wrap 统一渲染。
//
//   - rootLabels 按事件 ID 索引的根对象展示快照（文章标题/碎语摘要），缺失时回退为「ID xx」。
//   - brandName/siteURL 分别是邮件品牌名与站点前缀，留空时由 layout.ResolveBrand 兜底默认值。
func renderDigestHTML(tasks []model.NotificationEmailTask, events map[uint]model.NotificationEvent, rootLabels map[uint]string, brandName, siteURL string) string {
	brand := layout.ResolveBrand(brandName, siteURL)
	var b strings.Builder
	for _, task := range tasks {
		event := events[task.EventID]
		eventType := eventTypeOf(task, event)
		summary := html.EscapeString(eventSummary(event))
		b.WriteString(renderEventCard(eventType, event, rootLabels, brand.SiteURL, summary))
	}

	return layout.Wrap(layout.Options{
		Brand:    brand,
		Title:    fmt.Sprintf("你有 %d 条新的互动通知", len(tasks)),
		BodyHTML: template.HTML(b.String()),
		CTAText:  "查看全部通知",
		CTAURL:   brand.SiteURL,
	})
}

// renderEventCard 渲染单条通知卡片：类型徽章 + 具体场景文案。
func renderEventCard(eventType string, event model.NotificationEvent, rootLabels map[uint]string, siteURL, summary string) string {
	badge := badgeFor(eventType)
	line := renderEventLine(eventType, event, rootLabels, siteURL, summary)
	return fmt.Sprintf(
		`<div style="border:1px solid #EEE;border-radius:6px;padding:12px 14px;margin:0 0 10px;">`+
			`<span style="display:inline-block;font-size:11px;font-weight:700;color:%s;background:%s;border-radius:4px;padding:2px 8px;margin-bottom:6px;">%s</span>`+
			`<p style="font-size:14px;color:#374151;margin:0;line-height:1.5;">%s</p>`+
			`</div>`,
		badge.FgColor, badge.BgColor, html.EscapeString(badge.Label), line,
	)
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

// articleURL 在 rootID 非零时拼出 /articles/{id} 的可跳转链接（siteURL 由 layout.ResolveBrand 兜底，恒非空）。
func articleURL(rootID uint, siteURL string) string {
	if rootID == 0 {
		return ""
	}
	return siteURL + "/articles/" + strconv.FormatUint(uint64(rootID), 10)
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
