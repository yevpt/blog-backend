package imagecdn_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/imagecdn"
)

func TestObjectKeyFromCDNPath(t *testing.T) {
	key, err := imagecdn.ObjectKeyFromCDNPath("blog", "/blog/articles/cover.jpg")
	require.NoError(t, err)
	assert.Equal(t, "articles/cover.jpg", key)
}

func TestObjectKeyFromCDNPath_RejectsWrongBucket(t *testing.T) {
	_, err := imagecdn.ObjectKeyFromCDNPath("blog", "/other/x.jpg")
	require.Error(t, err)
}
