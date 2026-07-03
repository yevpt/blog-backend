package layout_test

import (
	"html/template"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/vpt/blog-backend/pkg/email/layout"
)

func TestResolveBrand_FallsBackToDefaultsWhenEmpty(t *testing.T) {
	brand := layout.ResolveBrand("", "")

	assert.Equal(t, layout.DefaultBrandName, brand.Name)
	assert.Equal(t, layout.DefaultSiteURL, brand.SiteURL)
}

func TestResolveBrand_KeepsConfiguredValuesAndTrimsTrailingSlash(t *testing.T) {
	brand := layout.ResolveBrand("  测试站  ", "https://test.example.com/")

	assert.Equal(t, "测试站", brand.Name)
	assert.Equal(t, "https://test.example.com", brand.SiteURL)
}

func TestWrap_RendersBrandTitleAndBody(t *testing.T) {
	html := layout.Wrap(layout.Options{
		Brand:    layout.Brand{Name: "YEVPT", SiteURL: "https://www.yevpt.com"},
		Title:    "注册验证码",
		BodyHTML: template.HTML(`<p>hello</p>`),
	})

	assert.Contains(t, html, `href="https://www.yevpt.com"`)
	assert.Contains(t, html, ">YEVPT<")
	assert.Contains(t, html, "注册验证码")
	assert.Contains(t, html, "<p>hello</p>")
	assert.Contains(t, html, "&copy;")
	assert.Contains(t, html, strconv.Itoa(time.Now().Year()))
	assert.NotContains(t, html, "打开审核后台") // 没有传 CTAText 时不渲染按钮
}

func TestWrap_RendersCTAButtonWhenProvided(t *testing.T) {
	html := layout.Wrap(layout.Options{
		Brand:    layout.Brand{Name: "YEVPT", SiteURL: "https://www.yevpt.com"},
		Title:    "标题",
		BodyHTML: template.HTML(`<p>正文</p>`),
		CTAText:  "查看全部通知",
		CTAURL:   "https://www.yevpt.com/notifications",
	})

	assert.Contains(t, html, "查看全部通知")
	assert.Contains(t, html, `href="https://www.yevpt.com/notifications"`)
}

func TestWrap_EscapesTitleButKeepsBodyHTMLRaw(t *testing.T) {
	html := layout.Wrap(layout.Options{
		Brand:    layout.Brand{Name: "YEVPT", SiteURL: "https://www.yevpt.com"},
		Title:    `<script>alert(1)</script>`,
		BodyHTML: template.HTML(`<p>安全内容</p>`),
	})

	assert.NotContains(t, html, "<script>alert(1)</script>")
	assert.Contains(t, html, "&lt;script&gt;")
	assert.Contains(t, html, "<p>安全内容</p>")
}
