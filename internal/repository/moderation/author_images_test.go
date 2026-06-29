package moderation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestAuthorOriginalImageViewsUsesSourceObjectKey(t *testing.T) {
	got := moderation.AuthorOriginalImageViews([]moderation.ImageView{{
		RevisionImageID: 1, Seq: 1,
		SourceObjectKey: "moments/cat.gif", DisplayObjectKey: "system/moderation/gif-review.jpg",
		Approved: false, IsGIF: true,
	}})

	requireLen := len(got)
	assert.Equal(t, 1, requireLen)
	assert.Equal(t, "moments/cat.gif", got[0].DisplayObjectKey)
	assert.True(t, got[0].Approved)
}
