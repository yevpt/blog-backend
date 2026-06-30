package textnorm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

func TestNormalizeCanonicalizesSharedClassificationText(t *testing.T) {
	raw := "違－禁ＡＡ"
	assert.Equal(t, "违禁a", textnorm.Normalize(raw))
}
