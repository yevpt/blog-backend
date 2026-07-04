package user

import (
	"context"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
	"github.com/vpt/blog-backend/pkg/storage"
)

const (
	managedAvatarListPrefix         = "avatar/user/"
	managedFriendLinkLogoListPrefix = "avatar/link/"
)

// storageObjectLister 列出对象存储前缀下的 key。
type storageObjectLister interface {
	ListObjectKeys(ctx context.Context, prefix string) ([]string, error)
}

// AvatarNormalizer 负责读取、检查并重压缩本站托管头像。
type AvatarNormalizer interface {
	LoadStoredAvatar(ctx context.Context, objectKey string) ([]byte, error)
	ReprocessStoredAvatar(ctx context.Context, data []byte, targetKey string) (avatarservice.SaveResult, error)
}

// FriendLinkLogoRefs 查询友链 Logo 在库内的引用计数。
type FriendLinkLogoRefs interface {
	CountByAvatarURL(avatarURL string) (int64, error)
}

// AdminDeps 管理端用户服务可选依赖。
type AdminDeps struct {
	Store      storage.ObjectStore
	Avatar     AvatarNormalizer
	FriendLink FriendLinkLogoRefs
	Moderation ModerationProfileReader
	Presence   OnlineChecker
}

func (s *adminService) NormalizeAvatars(ctx context.Context, req *dto.NormalizeAvatarsReq) (*dto.NormalizeAvatarsResp, error) {
	if s.avatar == nil || s.store == nil {
		return nil, ErrAvatarNormalizeUnavailable
	}
	if req == nil {
		req = &dto.NormalizeAvatarsReq{}
	}
	clearInvalid := req.ClearInvalid != nil && *req.ClearInvalid

	var users []model.User
	if req.UserID != nil {
		user, err := s.repo.FindByID(*req.UserID)
		if err != nil {
			return nil, err
		}
		if user == nil {
			return nil, ErrUserNotFound
		}
		users = []model.User{*user}
	} else {
		var err error
		users, err = s.repo.ListAllWithManagedAvatar()
		if err != nil {
			return nil, err
		}
	}

	resp := &dto.NormalizeAvatarsResp{
		Scanned: len(users),
		Items:   make([]dto.NormalizeAvatarItem, 0, len(users)),
	}
	processedKeys := make(map[string]struct{}, len(users))
	for i := range users {
		item := s.normalizeUserAvatar(ctx, &users[i], clearInvalid)
		resp.Items = append(resp.Items, item)
		trackProcessedAvatarKey(processedKeys, item)
		applyNormalizeItemCount(resp, item)
	}

	if req.UserID == nil {
		if err := s.sweepStorageAvatars(ctx, resp, processedKeys, clearInvalid); err != nil {
			return nil, err
		}
		if err := s.purgeDanglingAvatarObjects(ctx, resp); err != nil {
			return nil, err
		}
	} else if req.UserID != nil {
		if err := s.purgeDanglingAvatarObjects(ctx, resp); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (s *adminService) ClearUserAvatar(ctx context.Context, userID uint) (*dto.ClearUserAvatarResp, error) {
	if s.store == nil {
		return nil, ErrAvatarNormalizeUnavailable
	}
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	oldKey, ok := avatarservice.ResolveManagedAvatarKey(s.store, derefString(user.AvatarUrl))
	if !ok {
		return nil, ErrAvatarNotManaged
	}
	if err := s.repo.Update(userID, map[string]any{"avatar_url": nil}); err != nil {
		return nil, err
	}
	_ = s.cache.Invalidate(ctx, int64(userID))
	s.deleteUnreferencedAvatarObject(ctx, oldKey)
	return &dto.ClearUserAvatarResp{UserID: userID, OldKey: oldKey}, nil
}

func (s *adminService) sweepStorageAvatars(
	ctx context.Context,
	resp *dto.NormalizeAvatarsResp,
	processed map[string]struct{},
	clearInvalid bool,
) error {
	lister, ok := s.store.(storageObjectLister)
	if !ok {
		return nil
	}
	users, err := s.repo.ListAllWithManagedAvatar()
	if err != nil {
		return err
	}
	referenced := collectReferencedAvatarKeys(s.store, users)
	keys, err := lister.ListObjectKeys(ctx, managedAvatarListPrefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if !avatarservice.IsManagedAvatarKey(key) {
			continue
		}
		if _, done := processed[key]; done {
			continue
		}
		if _, ok := referenced[key]; !ok {
			continue
		}
		resp.StorageScanned++
		item := s.normalizeStorageAvatar(ctx, key, clearInvalid)
		resp.Items = append(resp.Items, item)
		trackProcessedAvatarKey(processed, item)
		applyNormalizeItemCount(resp, item)
	}
	return nil
}

func (s *adminService) purgeDanglingAvatarObjects(ctx context.Context, resp *dto.NormalizeAvatarsResp) error {
	lister, ok := s.store.(storageObjectLister)
	if !ok {
		return nil
	}
	users, err := s.repo.ListAllWithManagedAvatar()
	if err != nil {
		return err
	}
	referenced := collectReferencedAvatarKeys(s.store, users)
	if err := s.purgeUnreferencedObjects(ctx, resp, lister, managedAvatarListPrefix, func(key string) (bool, error) {
		_, ok := referenced[key]
		return ok, nil
	}); err != nil {
		return err
	}
	if s.friendLink == nil {
		return nil
	}
	return s.purgeUnreferencedObjects(ctx, resp, lister, managedFriendLinkLogoListPrefix, func(key string) (bool, error) {
		return s.isFriendLinkLogoKeyReferenced(ctx, key)
	})
}

func (s *adminService) purgeUnreferencedObjects(
	ctx context.Context,
	resp *dto.NormalizeAvatarsResp,
	lister storageObjectLister,
	prefix string,
	isReferenced func(string) (bool, error),
) error {
	keys, err := lister.ListObjectKeys(ctx, prefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if !isManagedObjectKeyUnderPrefix(key, prefix) {
			continue
		}
		referenced, err := isReferenced(key)
		if err != nil {
			return err
		}
		if referenced {
			continue
		}
		if err := s.store.DeleteObject(ctx, key); err != nil {
			resp.Failed++
			resp.Items = append(resp.Items, dto.NormalizeAvatarItem{
				OldKey:  key,
				Status:  "failed",
				Message: avatarservice.FormatNormalizeIssue(key, err.Error()),
			})
			continue
		}
		resp.Purged++
		resp.Items = append(resp.Items, dto.NormalizeAvatarItem{
			OldKey:  key,
			Status:  "cleared",
			Message: "已删除无引用的旧对象",
		})
	}
	return nil
}

func (s *adminService) isFriendLinkLogoKeyReferenced(_ context.Context, objectKey string) (bool, error) {
	if s.friendLink == nil || !isManagedObjectKeyUnderPrefix(objectKey, managedFriendLinkLogoListPrefix) {
		return false, nil
	}
	count, err := s.friendLink.CountByAvatarURL(objectKey)
	return count > 0, err
}

func isManagedObjectKeyUnderPrefix(key, prefix string) bool {
	key = strings.TrimSpace(key)
	return key != "" && strings.HasPrefix(key, prefix)
}

func (s *adminService) normalizeUserAvatar(ctx context.Context, user *model.User, clearInvalid bool) dto.NormalizeAvatarItem {
	item := dto.NormalizeAvatarItem{UserID: user.ID}
	if user.AvatarUrl == nil {
		item.Status = "skipped"
		item.Message = "未设置头像"
		return item
	}

	oldKey, ok := avatarservice.ResolveManagedAvatarKey(s.store, *user.AvatarUrl)
	if !ok {
		item.Status = "skipped"
		item.Message = "非本站托管头像"
		return item
	}
	item.OldKey = oldKey
	return s.normalizeAvatarObject(ctx, item, oldKey, clearInvalid, func(ctx context.Context, newKey string) error {
		if strings.TrimSpace(derefString(user.AvatarUrl)) == newKey {
			return nil
		}
		if err := s.repo.Update(user.ID, map[string]any{"avatar_url": newKey}); err != nil {
			return err
		}
		user.AvatarUrl = &newKey
		_ = s.cache.Invalidate(ctx, int64(user.ID))
		return nil
	})
}

func (s *adminService) normalizeStorageAvatar(ctx context.Context, objectKey string, clearInvalid bool) dto.NormalizeAvatarItem {
	item := dto.NormalizeAvatarItem{OldKey: objectKey, Message: "存储孤儿对象"}
	return s.normalizeAvatarObject(ctx, item, objectKey, clearInvalid, func(ctx context.Context, newKey string) error {
		if newKey == objectKey {
			return nil
		}
		if _, err := s.repo.ReplaceAvatarURL(objectKey, newKey); err != nil {
			return err
		}
		return nil
	})
}

func (s *adminService) normalizeAvatarObject(
	ctx context.Context,
	item dto.NormalizeAvatarItem,
	objectKey string,
	clearInvalid bool,
	persistKey func(context.Context, string) error,
) dto.NormalizeAvatarItem {
	data, err := s.avatar.LoadStoredAvatar(ctx, objectKey)
	if err != nil {
		if clearInvalid {
			return s.clearAvatarObject(ctx, item, objectKey)
		}
		item.Status = "failed"
		item.Message = avatarservice.FormatNormalizeIssue(objectKey, err.Error())
		return item
	}

	compliant, blockedReason := avatarservice.InspectStoredAvatar(data)
	if blockedReason != "" {
		if clearInvalid {
			return s.clearAvatarObject(ctx, item, objectKey)
		}
		item.Status = "failed"
		item.Message = avatarservice.FormatNormalizeIssue(objectKey, blockedReason)
		return item
	}
	if compliant {
		item.Status = "ok"
		return item
	}

	saved, err := s.avatar.ReprocessStoredAvatar(ctx, data, objectKey)
	if err != nil {
		if clearInvalid {
			return s.clearAvatarObject(ctx, item, objectKey)
		}
		item.Status = "failed"
		item.Message = avatarservice.FormatNormalizeIssue(objectKey, avatarservice.NormalizeFailureReason(err))
		return item
	}

	if err := persistKey(ctx, saved.ObjectKey); err != nil {
		if saved.Created {
			_ = s.store.DeleteObject(ctx, saved.ObjectKey)
		}
		item.Status = "failed"
		item.Message = avatarservice.FormatNormalizeIssue(objectKey, err.Error())
		return item
	}
	if saved.ObjectKey != objectKey {
		s.deleteUnreferencedAvatarObject(ctx, objectKey)
	}

	item.Status = "updated"
	item.NewKey = saved.ObjectKey
	if saved.ObjectKey == objectKey {
		item.Message = "已原地覆盖压缩"
	} else {
		item.Message = "已压缩并切换新 key"
	}
	return item
}

func (s *adminService) clearAvatarObject(ctx context.Context, item dto.NormalizeAvatarItem, objectKey string) dto.NormalizeAvatarItem {
	if item.UserID > 0 {
		return s.clearUserAvatarItem(ctx, item.UserID, objectKey)
	}
	if referenced, err := s.isAvatarKeyReferenced(ctx, objectKey); err != nil {
		item.Status = "failed"
		item.Message = avatarservice.FormatNormalizeIssue(objectKey, err.Error())
		return item
	} else if referenced {
		item.Status = "failed"
		item.Message = avatarservice.FormatNormalizeIssue(objectKey, "仍有用户引用，无法自动清除")
		return item
	}
	_ = s.store.DeleteObject(ctx, objectKey)
	item.Status = "cleared"
	item.Message = "已删除无引用的对象"
	return item
}

func (s *adminService) clearUserAvatarItem(ctx context.Context, userID uint, oldKey string) dto.NormalizeAvatarItem {
	item := dto.NormalizeAvatarItem{UserID: userID, OldKey: oldKey}
	if err := s.repo.Update(userID, map[string]any{"avatar_url": nil}); err != nil {
		item.Status = "failed"
		item.Message = avatarservice.FormatNormalizeIssue(oldKey, err.Error())
		return item
	}
	_ = s.cache.Invalidate(ctx, int64(userID))
	s.deleteUnreferencedAvatarObject(ctx, oldKey)
	item.Status = "cleared"
	item.Message = "已清空头像并删除对象"
	return item
}

func (s *adminService) deleteUnreferencedAvatarObject(ctx context.Context, objectKey string) {
	if objectKey == "" || s.store == nil || !avatarservice.IsManagedAvatarKey(objectKey) {
		return
	}
	referenced, err := s.isAvatarKeyReferenced(ctx, objectKey)
	if err != nil || referenced {
		return
	}
	_ = s.store.DeleteObject(ctx, objectKey)
}

func (s *adminService) isAvatarKeyReferenced(ctx context.Context, objectKey string) (bool, error) {
	users, err := s.repo.ListAllWithManagedAvatar()
	if err != nil {
		return false, err
	}
	_, ok := collectReferencedAvatarKeys(s.store, users)[objectKey]
	return ok, nil
}

func collectReferencedAvatarKeys(resolver storage.ObjectKeyResolver, users []model.User) map[string]struct{} {
	refs := make(map[string]struct{}, len(users))
	for i := range users {
		if users[i].AvatarUrl == nil {
			continue
		}
		raw := strings.TrimSpace(*users[i].AvatarUrl)
		if raw == "" {
			continue
		}
		if avatarservice.IsManagedAvatarKey(raw) {
			refs[raw] = struct{}{}
		}
		if key, ok := avatarservice.ResolveManagedAvatarKey(resolver, raw); ok {
			refs[key] = struct{}{}
		}
	}
	return refs
}

func trackProcessedAvatarKey(processed map[string]struct{}, item dto.NormalizeAvatarItem) {
	if item.NewKey != "" {
		processed[item.NewKey] = struct{}{}
	}
	if item.OldKey != "" && (item.NewKey == "" || item.NewKey == item.OldKey) {
		processed[item.OldKey] = struct{}{}
	}
}

func applyNormalizeItemCount(resp *dto.NormalizeAvatarsResp, item dto.NormalizeAvatarItem) {
	switch item.Status {
	case "updated":
		resp.Updated++
	case "cleared":
		resp.Cleared++
	case "skipped":
		resp.Skipped++
	case "ok":
		resp.OK++
	case "failed":
		resp.Failed++
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
