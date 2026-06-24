package user

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
)

func (s *userService) ChangeAvatar(userID uint, file *dto.UploadedImageFile) (*dto.UserDetailResp, error) {
	if file == nil || len(file.Data) == 0 {
		return nil, avatarservice.ErrAvatarInvalid
	}
	if s.avatar == nil || s.store == nil {
		return nil, avatarservice.ErrAvatarInvalid
	}

	ctx := context.Background()
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	oldAvatarURL := user.AvatarUrl
	saved, err := s.avatar.SaveUploadedAvatar(ctx, file.Name, file.Data)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Update(userID, map[string]any{"avatar_url": saved.ObjectKey}); err != nil {
		if saved.Created {
			_ = s.store.DeleteObject(ctx, saved.ObjectKey)
		}
		return nil, err
	}
	_ = s.cache.Invalidate(ctx, int64(userID))
	s.cleanupUnusedAvatar(ctx, oldAvatarURL)

	return s.cache.Get(ctx, int64(userID))
}

func (s *userService) cleanupUnusedAvatar(ctx context.Context, avatarURL *string) {
	if avatarURL == nil || s.store == nil {
		return
	}
	key := *avatarURL
	if !avatarservice.IsManagedAvatarKey(key) {
		return
	}
	count, err := s.repo.CountByAvatarURL(key)
	if err != nil || count > 0 {
		return
	}
	_ = s.store.DeleteObject(ctx, key)
}
