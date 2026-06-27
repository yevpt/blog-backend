package avatar_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
)

type keyResolver struct {
	key string
	err error
}

func (r keyResolver) ObjectKey(value string) (string, error) {
	return r.key, r.err
}

func TestResolveManagedAvatarKey_FromObjectKey(t *testing.T) {
	key, ok := avatarservice.ResolveManagedAvatarKey(nil, "avatar/user/abc.jpg")
	assert.True(t, ok)
	assert.Equal(t, "avatar/user/abc.jpg", key)
}

func TestResolveManagedAvatarKey_FromObjectURL(t *testing.T) {
	resolver := keyResolver{key: "avatar/user/abc.jpg"}
	key, ok := avatarservice.ResolveManagedAvatarKey(resolver, "https://cdn.example/avatar/user/abc.jpg")
	require.True(t, ok)
	assert.Equal(t, "avatar/user/abc.jpg", key)
}

func TestResolveManagedAvatarKey_RejectsExternalURL(t *testing.T) {
	resolver := keyResolver{key: "other/path.jpg"}
	_, ok := avatarservice.ResolveManagedAvatarKey(resolver, "https://example.com/other/path.jpg")
	assert.False(t, ok)
}
