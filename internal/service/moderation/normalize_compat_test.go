package moderation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

func TestCompatibilityWrapperMatchesSharedNormalizer(t *testing.T) {
	raw := "違－禁ＡＡ"
	assert.Equal(t, textnorm.Normalize(raw), moderation.NormalizeText(raw))
}
