package user

import (
	"context"

	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
)

// AvatarUploader 负责将头像文件保存到对象存储。
type AvatarUploader interface {
	SaveUploadedAvatar(ctx context.Context, name string, data []byte) (avatarservice.SaveResult, error)
}
