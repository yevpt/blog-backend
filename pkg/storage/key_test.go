package storage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/storage"
)

func TestObjectKeyFromValue_StripsBucketQueryAndHash(t *testing.T) {
	parser := storage.NewObjectKeyParser(storage.ObjectKeyParserConfig{
		Bucket:       "blog",
		AllowedHosts: []string{"cdn.example.com", "garage.example.com"},
	})

	key, err := parser.ObjectKey("https://cdn.example.com/blog/articles/45/images/a.png?sign=1#x")

	require.NoError(t, err)
	assert.Equal(t, "articles/45/images/a.png", key)
}

func TestObjectKeyFromValue_RejectsExternalHost(t *testing.T) {
	parser := storage.NewObjectKeyParser(storage.ObjectKeyParserConfig{
		Bucket:       "blog",
		AllowedHosts: []string{"cdn.example.com"},
	})

	_, err := parser.ObjectKey("https://evil.example.com/a.png")

	require.ErrorIs(t, err, storage.ErrExternalObjectURL)
	assert.Contains(t, err.Error(), "evil.example.com")
}

func TestObjectKeyFromValue_RejectsPathTraversal(t *testing.T) {
	parser := storage.NewObjectKeyParser(storage.ObjectKeyParserConfig{Bucket: "blog"})

	_, err := parser.ObjectKey("/blog/articles/45/../secret.png")

	require.ErrorIs(t, err, storage.ErrInvalidObjectKey)
}
