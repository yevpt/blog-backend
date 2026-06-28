package commentasset_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/service/commentasset"
)

type resolver struct{}

func (resolver) ObjectURL(_ context.Context, key string) (string, error) {
	return "https://cdn.example/" + key, nil
}

func TestResolveContentResolvesCommentAndModerationPreviewKeys(t *testing.T) {
	content := "![旧](comments/a.jpg) ![预览](moderation/previews/a.jpg)"

	got := commentasset.ResolveContent(context.Background(), resolver{}, content)

	assert.Equal(t, "![旧](https://cdn.example/comments/a.jpg) ![预览](https://cdn.example/moderation/previews/a.jpg)", got)
}
