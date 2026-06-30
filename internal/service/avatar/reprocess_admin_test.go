package avatar_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
)

func TestFormatNormalizeIssue_IncludesObjectKey(t *testing.T) {
	msg := avatarservice.FormatNormalizeIssue("avatar/user/broken.bin", "无法解码为有效图片")
	assert.Equal(t, "avatar/user/broken.bin：无法解码为有效图片", msg)
}

func TestNormalizeFailureReason_MapsUploadMessage(t *testing.T) {
	assert.Equal(t, "无法解码为有效图片", avatarservice.NormalizeFailureReason(avatarservice.ErrAvatarInvalid))
}

func TestInspectStoredAvatar_AcceptsCompliantPNG(t *testing.T) {
	compliant, blocked := avatarservice.InspectStoredAvatar(testPNGBytes(t, 80, 80))
	assert.True(t, compliant)
	assert.Empty(t, blocked)
}

func TestInspectStoredAvatar_ReprocessableGIF(t *testing.T) {
	compliant, blocked := avatarservice.InspectStoredAvatar(testGIF(t))
	assert.False(t, compliant)
	assert.Empty(t, blocked)
}
