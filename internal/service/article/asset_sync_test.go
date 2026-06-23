package article

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/storage"
)

func TestNormalizeArticleContent_CopiesTempImageAndRejectsExternal(t *testing.T) {
	store := &assetStore{
		keys: map[string]bool{
			"temp/articles/7/images/a.png": true,
		},
		keyMap: map[string]string{
			"https://cdn.example.com/blog/temp/articles/7/images/a.png?a=1": "temp/articles/7/images/a.png",
		},
	}

	result, err := normalizeArticleAssets(context.Background(), store, articleAssetNormalizeInput{
		ArticleID: 45,
		UserID:    7,
		Content:   "![a](https://cdn.example.com/blog/temp/articles/7/images/a.png?a=1)",
	})

	require.NoError(t, err)
	assert.Equal(t, "![a](articles/45/images/a.png)", result.Content)
	assert.Equal(t, []articleAssetCopy{{source: "temp/articles/7/images/a.png", target: "articles/45/images/a.png"}}, store.copies)
	assert.Equal(t, []string{"temp/articles/7/images/a.png"}, result.TempKeys)
}

func TestNormalizeArticleContent_RejectsExternalImage(t *testing.T) {
	_, err := normalizeArticleAssets(context.Background(), &assetStore{}, articleAssetNormalizeInput{
		ArticleID: 45,
		UserID:    7,
		Content:   "![bad](https://example.net/a.png)",
	})

	require.ErrorIs(t, err, ErrArticleImageExternal)
	assert.Contains(t, err.Error(), "https://example.net/a.png")
}

type assetStore struct {
	keys   map[string]bool
	keyMap map[string]string
	copies []articleAssetCopy
}

func (s *assetStore) ObjectURL(context.Context, string) (string, error) {
	return "", nil
}

func (s *assetStore) MoveObject(context.Context, string, string) error {
	return nil
}

func (s *assetStore) CopyObject(_ context.Context, sourceName string, targetName string) error {
	if s.keys == nil {
		s.keys = make(map[string]bool)
	}
	s.copies = append(s.copies, articleAssetCopy{source: sourceName, target: targetName})
	s.keys[targetName] = true
	return nil
}

func (s *assetStore) ObjectKey(value string) (string, error) {
	if value == "" {
		return "", storage.ErrInvalidObjectKey
	}
	if s.keyMap != nil {
		if key, ok := s.keyMap[value]; ok {
			return key, nil
		}
	}
	return "", storage.ErrExternalObjectURL
}

func (s *assetStore) ObjectExists(_ context.Context, objectName string) (bool, error) {
	if s.keys == nil {
		return false, nil
	}
	return s.keys[objectName], nil
}

func (s *assetStore) PutObject(context.Context, string, []byte, string) error {
	return nil
}

func (s *assetStore) DeleteObject(_ context.Context, objectName string) error {
	if s.keys != nil {
		delete(s.keys, objectName)
	}
	return nil
}
