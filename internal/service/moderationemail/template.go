package moderationemail

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
)

const (
	maxRenderedRows  = 50
	maxExcerptRunes  = 120
	adminModeration  = "/admin/moderation"
	reviewMailLayout = `<!doctype html>
<html lang="zh-CN">
<body>
<p>共 {{.TotalCount}} 条待审核内容。</p>
{{if .AdminURL}}<p><a href="{{.AdminURL}}">打开审核后台</a></p>{{else}}<p>请登录后台进入审核列表处理。</p>{{end}}
<table>
{{range .Rows}}<tr><td>{{.TypeLabel}}</td><td>#{{.ItemID}}</td><td>{{.Excerpt}}</td><td>{{.CreatedAt}}</td></tr>
{{end}}</table>
{{if gt .OverflowCount 0}}<p>另有 {{.OverflowCount}} 条未展示，请进入审核后台查看全部。</p>{{end}}
</body>
</html>`
)

var reviewEmailTemplate = template.Must(template.New("review-email").Parse(reviewMailLayout))

// RenderedEmail 是审核摘要邮件渲染结果。
type RenderedEmail struct {
	Subject string
	HTML    string
}

type reviewEmailData struct {
	TotalCount    int
	AdminURL      string
	Rows          []reviewEmailRow
	OverflowCount int
}

type reviewEmailRow struct {
	TypeLabel string
	ItemID    uint64
	Excerpt   string
	CreatedAt string
}

// Render 安全渲染审核摘要邮件；用户提交正文只作为模板数据输出，由 html/template 转义。
func Render(batch model.ModerationReviewEmailBatch, tasks []moderationemailrepo.PendingTask, siteURL string) (RenderedEmail, error) {
	// 以批次总数为权威来源，缺失时回退到任务数量。
	total := batch.ItemCount
	if total <= 0 {
		total = len(tasks)
	}

	// 只展示前 50 条，剩余数量通过溢出提示表达。
	displayed := minInt(len(tasks), maxRenderedRows)
	data := reviewEmailData{
		TotalCount:    total,
		AdminURL:      adminURL(siteURL),
		Rows:          renderRows(tasks[:displayed]),
		OverflowCount: maxInt(total-displayed, 0),
	}

	// 执行模板渲染，避免任何手写 HTML 拼接用户正文。
	var body bytes.Buffer
	if err := reviewEmailTemplate.Execute(&body, data); err != nil {
		return RenderedEmail{}, err
	}
	return RenderedEmail{
		Subject: fmt.Sprintf("待审核内容提醒（%d 条）", total),
		HTML:    body.String(),
	}, nil
}

func renderRows(tasks []moderationemailrepo.PendingTask) []reviewEmailRow {
	rows := make([]reviewEmailRow, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, reviewEmailRow{
			TypeLabel: typeLabel(task.ContentType),
			ItemID:    task.ItemID,
			Excerpt:   excerpt(task.SubmittedContent, maxExcerptRunes),
			CreatedAt: formatTime(task.CreatedAt),
		})
	}
	return rows
}

func adminURL(siteURL string) string {
	base := strings.TrimRight(strings.TrimSpace(siteURL), "/")
	if base == "" {
		return ""
	}
	return base + adminModeration
}

func typeLabel(contentType string) string {
	switch contentType {
	case model.ModerationContentMoment:
		return "碎语"
	case model.ModerationContentArticleComment:
		return "文章评论"
	case model.ModerationContentMomentComment:
		return "碎语评论"
	case model.ModerationContentGuestbook:
		return "留言"
	case model.ModerationContentArticleReply:
		return "文章回复"
	case model.ModerationContentMomentReply:
		return "碎语回复"
	case model.ModerationContentGuestbookReply:
		return "留言回复"
	default:
		return contentType
	}
}

func excerpt(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02 15:04")
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
