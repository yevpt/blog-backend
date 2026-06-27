package user_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
	user "github.com/vpt/blog-backend/internal/service/user"
)

type changeAvatarRepo struct {
	user         *model.User
	updateAvatar string
	countKey     string
	count        int64
}

func (r *changeAvatarRepo) FindByID(id uint) (*model.User, error) { return r.user, nil }
func (r *changeAvatarRepo) Update(id uint, updates map[string]any) error {
	if v, ok := updates["avatar_url"]; ok {
		r.updateAvatar = v.(string)
	}
	return nil
}
func (r *changeAvatarRepo) CountByAvatarURL(avatarURL string) (int64, error) {
	r.countKey = avatarURL
	return r.count, nil
}

func (r *changeAvatarRepo) FindByIdentifier(id string) (*model.User, error) { return nil, nil }
func (r *changeAvatarRepo) FindByUsername(username string) (*model.User, error) {
	return nil, nil
}
func (r *changeAvatarRepo) FindByEmail(email string) (*model.User, error) { return nil, nil }
func (r *changeAvatarRepo) FindDetailByID(id uint) (*userrepo.UserDetailAggregate, error) {
	return nil, nil
}
func (r *changeAvatarRepo) ListLikedContent(filter userrepo.LikedContentFilter) (*userrepo.LikedContentPageResult, error) {
	return nil, nil
}
func (r *changeAvatarRepo) CountLikedContent(userID uint) (int64, error) { return 0, nil }
func (r *changeAvatarRepo) ExistsByEmail(email string) (bool, error)     { return false, nil }
func (r *changeAvatarRepo) EmailInUseByOther(email string, excludeID uint) (bool, error) {
	return false, nil
}
func (r *changeAvatarRepo) ExistsByNickname(nickname string) (bool, error) { return false, nil }
func (r *changeAvatarRepo) Create(u *model.User, roleID uint) error        { return nil }
func (r *changeAvatarRepo) FindRolesByUserID(id uint) ([]string, error)    { return nil, nil }
func (r *changeAvatarRepo) FindRolesByUserIDs(ids []uint) (map[uint][]string, error) {
	return nil, nil
}
func (r *changeAvatarRepo) TouchLoginPresence(id uint) error  { return nil }
func (r *changeAvatarRepo) UpdateLastActiveAt(id uint) error { return nil }
func (r *changeAvatarRepo) UpdateLastLoginAt(id uint) error { return nil }
func (r *changeAvatarRepo) ListRecent(offset, limit int) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *changeAvatarRepo) ListAll(offset, limit int) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *changeAvatarRepo) DeleteSocialLink(userID uint, platform string) error { return nil }
func (r *changeAvatarRepo) ExistsByUsername(username string, excludeID uint) (bool, error) {
	return false, nil
}
func (r *changeAvatarRepo) UpdatePassword(userID uint, hashedPassword string) error  { return nil }
func (r *changeAvatarRepo) UpsertMeta(userID uint, updates map[string]any) error     { return nil }
func (r *changeAvatarRepo) UpsertSocialLink(userID uint, platform, url string) error { return nil }
func (r *changeAvatarRepo) UpsertUserSetting(userID uint, updates map[string]any) error {
	return nil
}
func (r *changeAvatarRepo) GrantVipRole(userID uint) error  { return nil }
func (r *changeAvatarRepo) RevokeVipRole(userID uint) error { return nil }
func (r *changeAvatarRepo) BatchFetchActiveLogin(ids []uint) (map[uint]*userrepo.ActiveLogin, error) {
	return nil, nil
}

type changeAvatarStore struct {
	deleted []string
}

func (s *changeAvatarStore) ObjectURL(ctx context.Context, objectName string) (string, error) {
	return "https://cdn.example.com/blog/" + objectName, nil
}
func (s *changeAvatarStore) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	return false, nil
}
func (s *changeAvatarStore) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	return nil
}
func (s *changeAvatarStore) MoveObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}
func (s *changeAvatarStore) CopyObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}
func (s *changeAvatarStore) ObjectKey(value string) (string, error) { return value, nil }
func (s *changeAvatarStore) DeleteObject(ctx context.Context, objectName string) error {
	s.deleted = append(s.deleted, objectName)
	return nil
}

type stubAvatarUploader struct {
	key     string
	created bool
}

func (s *stubAvatarUploader) SaveUploadedAvatar(ctx context.Context, name string, data []byte) (avatarservice.SaveResult, error) {
	return avatarservice.SaveResult{ObjectKey: s.key, Created: s.created}, nil
}

func TestUserService_ChangeAvatar_UpdatesAndCleansOldAvatar(t *testing.T) {
	old := "avatar/user/old.jpg"
	repo := &changeAvatarRepo{
		user:  &model.User{Base: model.Base{ID: 7}, AvatarUrl: &old},
		count: 0,
	}
	store := &changeAvatarStore{}
	cache := &stubUserCacheService{
		profile: &dto.UserDetailResp{ID: 7, Username: "alice"},
	}
	svc := user.NewUserService(cache, repo, store, &stubAvatarUploader{key: "avatar/user/new.jpg", created: true}, nil)

	resp, err := svc.ChangeAvatar(7, &dto.UploadedImageFile{Name: "a.png", Data: []byte("png")})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "avatar/user/new.jpg", repo.updateAvatar)
	assert.Equal(t, old, repo.countKey)
	assert.Equal(t, []string{old}, store.deleted)
}

func TestUserService_ChangeAvatar_KeepsSharedOldAvatar(t *testing.T) {
	old := "avatar/user/shared.jpg"
	repo := &changeAvatarRepo{
		user:  &model.User{Base: model.Base{ID: 7}, AvatarUrl: &old},
		count: 2,
	}
	store := &changeAvatarStore{}
	cache := &stubUserCacheService{
		profile: &dto.UserDetailResp{ID: 7, Username: "alice"},
	}
	svc := user.NewUserService(cache, repo, store, &stubAvatarUploader{key: "avatar/user/new.jpg"}, nil)

	_, err := svc.ChangeAvatar(7, &dto.UploadedImageFile{Name: "a.png", Data: []byte("png")})
	require.NoError(t, err)
	assert.Empty(t, store.deleted)
}
