package moderationemail

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
	"github.com/vpt/blog-backend/pkg/email/layout"
)

const (
	maxRenderedRows = 50
	maxExcerptRunes = 120
	// defaultAdminURL 是 admin_url 留空时的兜底审核后台地址，直接作为最终跳转地址使用。
	defaultAdminURL = "https://admin.yevpt.com"
)

// reviewCardsLayout 是审核提醒邮件的正文片段（不含外层品牌壳体）：
// 每条待审内容渲染成一张卡片，超出展示上限时在最前面提示溢出条数。
const reviewCardsLayout = `{{if .OverflowCount}}<p style="font-size:13px;color:#6B7280;margin:0 0 16px;">以下展示前 {{len .Rows}} 条，其余 {{.OverflowCount}} 条请前往后台查看。</p>{{end}}
{{range .Rows}}<div style="border:1px solid #EEE;border-radius:6px;padding:12px 14px;margin:0 0 10px;">
<span style="display:inline-block;font-size:11px;font-weight:700;color:#534AB7;background:#EEEDFE;border-radius:4px;padding:2px 8px;margin-bottom:6px;">{{.TypeLabel}}</span>
<p style="font-size:14px;color:#374151;margin:0 0 4px;line-height:1.5;">{{.Excerpt}}</p>
<p style="font-size:12px;color:#9CA3AF;margin:0;">#{{.ItemID}} · 作者 #{{.AuthorID}} · {{.CreatedAt}}</p>
</div>
{{end}}`

var reviewCardsTemplate = template.Must(template.New("review-cards").Parse(reviewCardsLayout))

// RenderedEmail 是审核摘要邮件渲染结果。
type RenderedEmail struct {
	Subject string
	HTML    string
}

type reviewEmailData struct {
	Rows          []reviewEmailRow
	OverflowCount int
}

type reviewEmailRow struct {
	TypeLabel string
	ItemID    uint64
	AuthorID  uint64
	Excerpt   string
	CreatedAt string
}

// Render 安全渲染审核摘要邮件；用户提交正文只作为模板数据输出，由 html/template 转义。
// brandName/siteURL 用于顶部品牌 Logo，adminURL 留空时使用 defaultAdminURL，非空时直接作为
// 「打开审核后台」按钮的最终跳转地址，不拼接路径。
func Render(batch model.ModerationReviewEmailBatch, tasks []moderationemailrepo.PendingTask, siteURL, brandName, adminURL string) (RenderedEmail, error) {
	// 以批次总数为权威来源，缺失时回退到任务数量。
	total := batch.ItemCount
	if total <= 0 {
		total = len(tasks)
	}

	// 只展示前 50 条，剩余数量通过溢出提示表达。
	displayed := minInt(len(tasks), maxRenderedRows)
	data := reviewEmailData{
		Rows:          renderRows(tasks[:displayed]),
		OverflowCount: maxInt(total-displayed, 0),
	}

	// 执行模板渲染，避免任何手写 HTML 拼接用户正文。
	var body bytes.Buffer
	if err := reviewCardsTemplate.Execute(&body, data); err != nil {
		return RenderedEmail{}, err
	}

	brand := layout.ResolveBrand(brandName, siteURL)
	htmlBody := layout.Wrap(layout.Options{
		Brand:    brand,
		Title:    fmt.Sprintf("共 %d 条待审核内容", total),
		BodyHTML: template.HTML(body.String()),
		CTAText:  "打开审核后台",
		CTAURL:   resolveAdminURL(adminURL),
	})

	return RenderedEmail{
		Subject: fmt.Sprintf("待审核内容提醒（%d 条）", total),
		HTML:    htmlBody,
	}, nil
}

// resolveAdminURL 对审核后台地址应用兜底默认值。
func resolveAdminURL(adminURL string) string {
	adminURL = strings.TrimSpace(adminURL)
	if adminURL == "" {
		return defaultAdminURL
	}
	return adminURL
}

func renderRows(tasks []moderationemailrepo.PendingTask) []reviewEmailRow {
	rows := make([]reviewEmailRow, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, reviewEmailRow{
			TypeLabel: typeLabel(task.ContentType),
			ItemID:    task.ItemID,
			AuthorID:  task.AuthorID,
			Excerpt:   excerpt(task.SubmittedContent, maxExcerptRunes),
			CreatedAt: formatTime(task.CreatedAt),
		})
	}
	return rows
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
