package user_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/chai2010/webp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
	user "github.com/vpt/blog-backend/internal/service/user"
)

type normalizeAvatarRepo struct {
	user         *model.User
	users        []model.User
	updateAvatar string
	countKey     string
	count        int64
}

func (r *normalizeAvatarRepo) ReplaceAvatarURL(oldURL, newURL string) (int64, error) {
	var affected int64
	for i := range r.users {
		if r.users[i].AvatarUrl != nil && *r.users[i].AvatarUrl == oldURL {
			value := newURL
			r.users[i].AvatarUrl = &value
			affected++
		}
	}
	if r.user != nil && r.user.AvatarUrl != nil && *r.user.AvatarUrl == oldURL {
		value := newURL
		r.user.AvatarUrl = &value
		if affected == 0 {
			affected = 1
		}
	}
	if affected > 0 {
		r.updateAvatar = newURL
	}
	return affected, nil
}

func (r *normalizeAvatarRepo) ListAllWithManagedAvatar() ([]model.User, error) {
	return r.users, nil
}
func (r *normalizeAvatarRepo) FindByID(id uint) (*model.User, error) {
	if r.user != nil && r.user.ID == id {
		return r.user, nil
	}
	for i := range r.users {
		if r.users[i].ID == id {
			user := r.users[i]
			return &user, nil
		}
	}
	return nil, nil
}
func (r *normalizeAvatarRepo) Update(id uint, updates map[string]any) error {
	if v, ok := updates["avatar_url"]; ok {
		if v == nil {
			r.updateAvatar = ""
			if r.user != nil && r.user.ID == id {
				r.user.AvatarUrl = nil
			}
			for i := range r.users {
				if r.users[i].ID == id {
					r.users[i].AvatarUrl = nil
				}
			}
		} else {
			value := v.(string)
			r.updateAvatar = value
			if r.user != nil && r.user.ID == id {
				r.user.AvatarUrl = &value
			}
			for i := range r.users {
				if r.users[i].ID == id {
					r.users[i].AvatarUrl = &value
				}
			}
		}
	}
	return nil
}
func (r *normalizeAvatarRepo) CountByAvatarURL(avatarURL string) (int64, error) {
	r.countKey = avatarURL
	return r.count, nil
}

func (r *normalizeAvatarRepo) FindByIdentifier(id string) (*model.User, error) { return nil, nil }
func (r *normalizeAvatarRepo) FindByUsername(username string) (*model.User, error) {
	return nil, nil
}
func (r *normalizeAvatarRepo) FindByEmail(email string) (*model.User, error) { return nil, nil }
func (r *normalizeAvatarRepo) FindDetailByID(id uint) (*userrepo.UserDetailAggregate, error) {
	return nil, nil
}
func (r *normalizeAvatarRepo) ListLikedContent(filter userrepo.LikedContentFilter) (*userrepo.LikedContentPageResult, error) {
	return nil, nil
}
func (r *normalizeAvatarRepo) CountLikedContent(userID uint) (int64, error) { return 0, nil }
func (r *normalizeAvatarRepo) ExistsByEmail(email string) (bool, error)     { return false, nil }
func (r *normalizeAvatarRepo) EmailInUseByOther(email string, excludeID uint) (bool, error) {
	return false, nil
}
func (r *normalizeAvatarRepo) ExistsByNickname(nickname string) (bool, error) { return false, nil }
func (r *normalizeAvatarRepo) Create(u *model.User, roleID uint) error        { return nil }
func (r *normalizeAvatarRepo) FindRolesByUserID(id uint) ([]string, error)    { return nil, nil }
func (r *normalizeAvatarRepo) FindRolesByUserIDs(ids []uint) (map[uint][]string, error) {
	return nil, nil
}
func (r *normalizeAvatarRepo) TouchLoginPresence(id uint) error  { return nil }
func (r *normalizeAvatarRepo) UpdateLastActiveAt(id uint) error { return nil }
func (r *normalizeAvatarRepo) UpdateLastLoginAt(id uint) error  { return nil }
func (r *normalizeAvatarRepo) ListRecent(offset, limit int) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *normalizeAvatarRepo) ListAll(offset, limit int) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *normalizeAvatarRepo) DeleteSocialLink(userID uint, platform string) error { return nil }
func (r *normalizeAvatarRepo) ExistsByUsername(username string, excludeID uint) (bool, error) {
	return false, nil
}
func (r *normalizeAvatarRepo) UpdatePassword(userID uint, hashedPassword string) error  { return nil }
func (r *normalizeAvatarRepo) UpsertMeta(userID uint, updates map[string]any) error     { return nil }
func (r *normalizeAvatarRepo) UpsertSocialLink(userID uint, platform, url string) error { return nil }
func (r *normalizeAvatarRepo) UpsertUserSetting(userID uint, updates map[string]any) error {
	return nil
}
func (r *normalizeAvatarRepo) GrantVipRole(userID uint) error  { return nil }
func (r *normalizeAvatarRepo) RevokeVipRole(userID uint) error { return nil }
func (r *normalizeAvatarRepo) BatchFetchActiveLogin(ids []uint) (map[uint]*userrepo.ActiveLogin, error) {
	return nil, nil
}

type normalizeAvatarStore struct {
	deleted  []string
	objects  map[string][]byte
	listKeys []string
}

func (s *normalizeAvatarStore) ObjectURL(ctx context.Context, objectName string) (string, error) {
	return "", nil
}
func (s *normalizeAvatarStore) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	if s.objects == nil {
		return true, nil
	}
	_, ok := s.objects[objectName]
	return ok, nil
}
func (s *normalizeAvatarStore) GetObject(ctx context.Context, objectName string) ([]byte, error) {
	if s.objects == nil {
		return nil, nil
	}
	return s.objects[objectName], nil
}
func (s *normalizeAvatarStore) ListObjectKeys(ctx context.Context, prefix string) ([]string, error) {
	return append([]string(nil), s.listKeys...), nil
}
func (s *normalizeAvatarStore) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	return nil
}
func (s *normalizeAvatarStore) MoveObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}
func (s *normalizeAvatarStore) CopyObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}
func (s *normalizeAvatarStore) ObjectKey(value string) (string, error) {
	const prefix = "https://cdn.example/"
	if strings.HasPrefix(value, prefix) {
		return strings.TrimPrefix(value, prefix), nil
	}
	return value, nil
}
func (s *normalizeAvatarStore) DeleteObject(ctx context.Context, objectName string) error {
	s.deleted = append(s.deleted, objectName)
	return nil
}

type stubAvatarNormalizer struct {
	data      []byte
	loadErr   error
	newKey    string
	created   bool
	reprocess bool
}

func (s *stubAvatarNormalizer) LoadStoredAvatar(ctx context.Context, objectKey string) ([]byte, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.data, nil
}

func (s *stubAvatarNormalizer) ReprocessStoredAvatar(ctx context.Context, data []byte, targetKey string) (avatarservice.SaveResult, error) {
	s.reprocess = true
	key := s.newKey
	if key == "" {
		key = targetKey
	}
	return avatarservice.SaveResult{ObjectKey: key, Created: s.created}, nil
}

func TestAdminService_NormalizeAvatars_UpdatesNonCompliantAvatar(t *testing.T) {
	old := "avatar/user/old.png"
	repo := &normalizeAvatarRepo{
		users: []model.User{{Base: model.Base{ID: 3}, AvatarUrl: &old}},
		count: 0,
	}
	store := &normalizeAvatarStore{}
	normalizer := &stubAvatarNormalizer{
		data:    testOversizedPNG(t),
		newKey:  "avatar/user/new.jpg",
		created: true,
	}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  store,
		Avatar: normalizer,
	})

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, resp.Scanned)
	assert.Equal(t, 1, resp.Updated)
	assert.Equal(t, "avatar/user/new.jpg", repo.updateAvatar)
	assert.Contains(t, store.deleted, old)
	assert.True(t, normalizer.reprocess)
}

func TestAdminService_NormalizeAvatars_PurgesUnreferencedStorageObjects(t *testing.T) {
	old := "avatar/user/orphan.png"
	repo := &normalizeAvatarRepo{users: nil}
	store := &normalizeAvatarStore{
		listKeys: []string{old},
		objects:  map[string][]byte{old: testOversizedPNG(t)},
	}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  store,
		Avatar: &stubAvatarNormalizer{},
	})

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{})
	require.NoError(t, err)
	assert.Equal(t, 0, resp.StorageScanned)
	assert.Equal(t, 1, resp.Purged)
	assert.Contains(t, store.deleted, old)
}

func TestAdminService_NormalizeAvatars_PurgesDanglingObjects(t *testing.T) {
	newKey := "avatar/user/new.jpg"
	oldOrphan := "avatar/user/old-large.png"
	repo := &normalizeAvatarRepo{
		users: []model.User{{Base: model.Base{ID: 1}, AvatarUrl: &newKey}},
		count: 0,
	}
	store := &normalizeAvatarStore{
		listKeys: []string{newKey, oldOrphan},
		objects: map[string][]byte{
			newKey:    testSmallJPEG(t),
			oldOrphan: testOversizedPNG(t),
		},
	}
	normalizer := &stubAvatarNormalizer{data: testSmallJPEG(t)}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  store,
		Avatar: normalizer,
	})

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Purged)
	assert.Contains(t, store.deleted, oldOrphan)
}

func TestAdminService_NormalizeAvatars_DeletesOldKeyAfterExtensionChange(t *testing.T) {
	old := "avatar/user/foo.png"
	newKey := "avatar/user/foo.jpg"
	repo := &normalizeAvatarRepo{
		users: []model.User{{Base: model.Base{ID: 2}, AvatarUrl: &old}},
		count: 0,
	}
	store := &normalizeAvatarStore{
		listKeys: []string{old, newKey},
		objects: map[string][]byte{
			old:    testOversizedPNG(t),
			newKey: testSmallJPEG(t),
		},
	}
	normalizer := &stubAvatarNormalizer{
		data:   testOversizedPNG(t),
		newKey: newKey,
	}
	userID := uint(2)
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  store,
		Avatar: normalizer,
	})

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{UserID: &userID})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Updated)
	assert.Equal(t, newKey, repo.updateAvatar)
	assert.Contains(t, store.deleted, old)
}

func TestAdminService_NormalizeAvatars_FailureIncludesObjectKey(t *testing.T) {
	old := "avatar/user/broken.bin"
	repo := &normalizeAvatarRepo{
		users: []model.User{{Base: model.Base{ID: 8}, AvatarUrl: &old}},
	}
	normalizer := &stubAvatarNormalizer{data: []byte("not-an-image")}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  &normalizeAvatarStore{},
		Avatar: normalizer,
	})

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "failed", resp.Items[0].Status)
	assert.Equal(t, old, resp.Items[0].OldKey)
	assert.Contains(t, resp.Items[0].Message, old)
	assert.Contains(t, resp.Items[0].Message, "无法解码")
}

func TestAdminService_NormalizeAvatars_ClearInvalidOnFailure(t *testing.T) {
	old := "avatar/user/broken.bin"
	repo := &normalizeAvatarRepo{
		users: []model.User{{Base: model.Base{ID: 8}, AvatarUrl: &old}},
		count: 0,
	}
	store := &normalizeAvatarStore{}
	normalizer := &stubAvatarNormalizer{data: []byte("not-an-image")}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  store,
		Avatar: normalizer,
	})
	clearInvalid := true

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{ClearInvalid: &clearInvalid})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Cleared)
	assert.Equal(t, "cleared", resp.Items[0].Status)
	assert.Equal(t, []string{old}, store.deleted)
}

func TestAdminService_ClearUserAvatar_RemovesManagedAvatar(t *testing.T) {
	old := "avatar/user/old.jpg"
	repo := &normalizeAvatarRepo{
		user: &model.User{Base: model.Base{ID: 9}, AvatarUrl: &old},
	}
	store := &normalizeAvatarStore{}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{Store: store})

	resp, err := svc.ClearUserAvatar(context.Background(), 9)
	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.UserID)
	assert.Equal(t, old, resp.OldKey)
	assert.Equal(t, []string{old}, store.deleted)
}

func TestAdminService_NormalizeAvatars_ResolvesCDNURL(t *testing.T) {
	oldURL := "https://cdn.example/avatar/user/cdn-old.png"
	oldKey := "avatar/user/cdn-old.png"
	repo := &normalizeAvatarRepo{
		users: []model.User{{Base: model.Base{ID: 5}, AvatarUrl: &oldURL}},
		count: 0,
	}
	store := &normalizeAvatarStore{}
	normalizer := &stubAvatarNormalizer{
		data:    testOversizedPNG(t),
		newKey:  "avatar/user/cdn-new.jpg",
		created: true,
	}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  store,
		Avatar: normalizer,
	})

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Updated)
	assert.Equal(t, oldKey, resp.Items[0].OldKey)
	assert.Equal(t, "avatar/user/cdn-new.jpg", repo.updateAvatar)
}

func TestAdminService_NormalizeAvatars_InPlaceOverwriteSameKey(t *testing.T) {
	old := "avatar/user/same.jpg"
	repo := &normalizeAvatarRepo{
		users: []model.User{{Base: model.Base{ID: 4}, AvatarUrl: &old}},
	}
	normalizer := &stubAvatarNormalizer{
		data:    testOversizedPNG(t),
		newKey:  old,
		created: true,
	}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  &normalizeAvatarStore{},
		Avatar: normalizer,
	})

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Updated)
	assert.Equal(t, "updated", resp.Items[0].Status)
	assert.Equal(t, old, resp.Items[0].NewKey)
	assert.Contains(t, resp.Items[0].Message, "原地覆盖")
	assert.True(t, normalizer.reprocess)
}

func TestAdminService_NormalizeAvatars_SkipsCompliantAvatar(t *testing.T) {
	old := "avatar/user/ok.jpg"
	userID := uint(3)
	data := testSmallWebP(t)
	ok, err := avatarservice.IsCompliantAvatarData(data)
	require.NoError(t, err)
	require.True(t, ok)

	repo := &normalizeAvatarRepo{
		user: &model.User{Base: model.Base{ID: userID}, AvatarUrl: &old},
	}
	normalizer := &stubAvatarNormalizer{data: data}
	svc := user.NewAdminService(repo, &stubUserCacheService{}, user.AdminDeps{
		Store:  &normalizeAvatarStore{},
		Avatar: normalizer,
	})

	resp, err := svc.NormalizeAvatars(context.Background(), &dto.NormalizeAvatarsReq{UserID: &userID})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.OK)
	assert.Equal(t, 0, resp.Updated)
	assert.False(t, normalizer.reprocess)
}

func testSmallWebP(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: 80}))
	return buf.Bytes()
}

func testSmallJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}))
	return buf.Bytes()
}

func testOversizedPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
