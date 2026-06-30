package moderation

import "github.com/vpt/blog-backend/internal/service/moderation/textnorm"

// NormalizeText 仅为分类生成稳定文本，不修改对外展示正文。
func NormalizeText(text string) string {
	return textnorm.Normalize(text)
}
