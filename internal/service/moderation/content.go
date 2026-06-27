package moderation

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

type contentProcessor struct {
	policy *bluemonday.Policy
}

// NewContentProcessor 创建使用显式安全标签和协议白名单的正文处理器。
func NewContentProcessor() ContentProcessor {
	policy := bluemonday.NewPolicy()
	policy.AllowElements(
		"p", "br", "strong", "em", "b", "i", "u", "s",
		"blockquote", "code", "pre", "ul", "ol", "li", "a",
	)
	policy.AllowAttrs("href", "title").OnElements("a")
	policy.AllowURLSchemes("http", "https", "mailto")
	policy.RequireParseableURLs(true)
	policy.SkipElementsContent("script", "style", "iframe", "object", "embed", "svg", "math")

	return &contentProcessor{policy: policy}
}

func (p *contentProcessor) Process(raw string, limit int) (ProcessedContent, error) {
	published := strings.TrimSpace(p.policy.Sanitize(raw))
	plainText, links := extractTextAndLinks(published)
	if limit >= 0 && utf8.RuneCountInString(plainText) > limit {
		return ProcessedContent{}, ErrContentTooLong
	}

	return ProcessedContent{
		Published: published,
		PlainText: plainText,
		Links:     links,
	}, nil
}

func extractTextAndLinks(content string) (string, []string) {
	root, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return "", nil
	}

	var text strings.Builder
	links := make([]string, 0)
	walkHTML(root, &text, &links)
	return strings.TrimSpace(text.String()), links
}

func walkHTML(node *html.Node, text *strings.Builder, links *[]string) {
	if node.Type == html.TextNode {
		text.WriteString(node.Data)
	}
	if node.Type == html.ElementNode && node.Data == "a" {
		appendSafeLink(node, links)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, text, links)
	}
}

func appendSafeLink(node *html.Node, links *[]string) {
	for _, attribute := range node.Attr {
		if attribute.Key != "href" {
			continue
		}
		parsed, err := url.Parse(attribute.Val)
		if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https" || parsed.Scheme == "mailto") {
			*links = append(*links, attribute.Val)
		}
		return
	}
}
