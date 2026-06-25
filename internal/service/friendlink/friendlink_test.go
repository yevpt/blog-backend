package friendlink_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	friendlinkrepo "github.com/vpt/blog-backend/internal/repository/friendlink"
	"github.com/vpt/blog-backend/internal/service/friendlink"
)

type fakeFriendLinkRepository struct {
	listPublicOffset int
	listPublicLimit  int
	listPublicLinks  []model.FriendLink
	listPublicTotal  int64

	getPublicLink *model.FriendLink

	createGot  model.FriendLink
	createResp *model.FriendLink
	createErr  error

	getAdminLink *model.FriendLink
	updateID     uint
	updateData   friendlinkrepo.FriendLinkUpdateData
	updateResp   *model.FriendLink
	updateErr    error

	countByAvatarURL map[string]int64
}

func (r *fakeFriendLinkRepository) ListPublic(offset, limit int) ([]model.FriendLink, int64, error) {
	r.listPublicOffset = offset
	r.listPublicLimit = limit
	return r.listPublicLinks, r.listPublicTotal, nil
}

func (r *fakeFriendLinkRepository) GetPublic(id uint) (*model.FriendLink, error) {
	return r.getPublicLink, nil
}

func (r *fakeFriendLinkRepository) GetAdmin(id uint) (*model.FriendLink, error) {
	return r.getAdminLink, nil
}

func (r *fakeFriendLinkRepository) ListAdmin(offset, limit int, status *uint8) ([]model.FriendLink, int64, error) {
	return nil, 0, nil
}

func (r *fakeFriendLinkRepository) Create(link model.FriendLink) (*model.FriendLink, error) {
	r.createGot = link
	if r.createErr != nil {
		return nil, r.createErr
	}
	if r.createResp != nil {
		return r.createResp, nil
	}
	link.ID = 9
	return &link, nil
}

func (r *fakeFriendLinkRepository) Update(id uint, data friendlinkrepo.FriendLinkUpdateData) (*model.FriendLink, error) {
	r.updateID = id
	r.updateData = data
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	return r.updateResp, nil
}

func (r *fakeFriendLinkRepository) Delete(id uint) (*model.FriendLink, error) {
	return nil, nil
}

func (r *fakeFriendLinkRepository) CountByAvatarURL(avatarURL string) (int64, error) {
	if r.countByAvatarURL == nil {
		return 0, nil
	}
	return r.countByAvatarURL[avatarURL], nil
}

type fakeFriendLinkResolver struct {
	urls map[string]string
	got  []string
}

func (r *fakeFriendLinkResolver) ObjectURL(ctx context.Context, objectName string) (string, error) {
	r.got = append(r.got, objectName)
	return r.urls[objectName], nil
}

type fakeFriendLinkObjectStore struct {
	exists    bool
	puts      []friendLinkPutRecord
	deletes   []string
	urls      map[string]string
	objectURL []string
}

type friendLinkPutRecord struct {
	key         string
	data        []byte
	contentType string
}

func (s *fakeFriendLinkObjectStore) ObjectURL(ctx context.Context, objectName string) (string, error) {
	s.objectURL = append(s.objectURL, objectName)
	if s.urls != nil {
		if url, ok := s.urls[objectName]; ok {
			return url, nil
		}
	}
	return "https://cdn.example.com/blog/" + objectName, nil
}

func (s *fakeFriendLinkObjectStore) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	return s.exists, nil
}

func (s *fakeFriendLinkObjectStore) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	s.puts = append(s.puts, friendLinkPutRecord{
		key:         objectName,
		data:        append([]byte(nil), data...),
		contentType: contentType,
	})
	return nil
}

func (s *fakeFriendLinkObjectStore) DeleteObject(ctx context.Context, objectName string) error {
	s.deletes = append(s.deletes, objectName)
	return nil
}

func (s *fakeFriendLinkObjectStore) MoveObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}

func (s *fakeFriendLinkObjectStore) CopyObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}

func (s *fakeFriendLinkObjectStore) ObjectKey(value string) (string, error) {
	return value, nil
}

func TestFriendLinkService_ListPublic_ResolvesAvatarURL(t *testing.T) {
	avatar := "friend/avatar.png"
	repo := &fakeFriendLinkRepository{
		listPublicLinks: []model.FriendLink{
			{
				Base:      model.Base{ID: 1, CreatedAt: time.Unix(10, 0), UpdatedAt: time.Unix(20, 0)},
				Name:      "友站",
				Site:      "https://friend.example.com",
				AvatarUrl: &avatar,
				Seq:       2,
				Status:    1,
			},
		},
		listPublicTotal: 1,
	}
	resolver := &fakeFriendLinkResolver{urls: map[string]string{
		avatar: "https://cdn.example.com/blog/friend/avatar.png?sign=1",
	}}
	svc := friendlink.NewFriendLinkService(repo, resolver)

	resp, err := svc.ListPublic(dto.FriendLinkListReq{Page: 0, PageSize: 99})
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, 0, repo.listPublicOffset)
	assert.Equal(t, 50, repo.listPublicLimit)
	assert.Equal(t, 1, resp.Pages)
	assert.Equal(t, resolver.urls[avatar], *resp.List[0].AvatarUrl)
	assert.Equal(t, []string{avatar}, resolver.got)
}

func TestFriendLinkService_GetPublic_HiddenLinkReturnsNotFound(t *testing.T) {
	repo := &fakeFriendLinkRepository{}
	svc := friendlink.NewFriendLinkService(repo, nil)

	_, err := svc.GetPublic(3)
	require.ErrorIs(t, err, friendlink.ErrFriendLinkNotFound)
}

func TestFriendLinkService_Create_DefaultsStatusAndTrimsFields(t *testing.T) {
	seq := uint(4)
	repo := &fakeFriendLinkRepository{}
	store := &fakeFriendLinkObjectStore{}
	svc := friendlink.NewFriendLinkService(repo, store)

	resp, err := svc.Create(dto.FriendLinkCreateReq{
		Name: "  友站  ",
		Site: " https://friend.example.com ",
		Seq:  &seq,
		Logo: &dto.UploadedImageFile{Name: "logo.png", Data: friendLinkPNG(t, 240, 200)},
	})
	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.ID)
	assert.Equal(t, "友站", repo.createGot.Name)
	assert.Equal(t, "https://friend.example.com", repo.createGot.Site)
	require.NotNil(t, repo.createGot.AvatarUrl)
	assert.Regexp(t, `^avatar/link/[a-f0-9]{32}\.jpg$`, *repo.createGot.AvatarUrl)
	assert.Equal(t, uint8(1), repo.createGot.Status)
	require.Len(t, store.puts, 1)
	assert.Equal(t, *repo.createGot.AvatarUrl, store.puts[0].key)
	assert.Equal(t, "image/jpeg", store.puts[0].contentType)
}

func TestFriendLinkService_Update_AllowsClearingOptionalFields(t *testing.T) {
	name := " 新友站 "
	oldAvatar := "avatar/link/old.jpg"
	repo := &fakeFriendLinkRepository{
		getAdminLink: &model.FriendLink{
			Base:      model.Base{ID: 7},
			Name:      "旧友站",
			Site:      "https://old.example.com",
			AvatarUrl: &oldAvatar,
			Seq:       1,
			Status:    1,
		},
		updateResp: &model.FriendLink{
			Base:   model.Base{ID: 7},
			Name:   "新友站",
			Site:   "https://friend.example.com",
			Seq:    1,
			Status: 1,
		},
	}
	store := &fakeFriendLinkObjectStore{}
	svc := friendlink.NewFriendLinkService(repo, store)

	resp, err := svc.Update(7, dto.FriendLinkUpdateReq{
		Name: &name,
		Logo: &dto.UploadedImageFile{Name: "logo.png", Data: friendLinkPNG(t, 120, 120)},
	})
	require.NoError(t, err)
	assert.Equal(t, uint(7), resp.ID)
	assert.Equal(t, uint(7), repo.updateID)
	require.NotNil(t, repo.updateData.Name)
	assert.Equal(t, "新友站", *repo.updateData.Name)
	assert.True(t, repo.updateData.UpdateAvatarUrl)
	require.NotNil(t, repo.updateData.AvatarUrl)
	assert.Regexp(t, `^avatar/link/[a-f0-9]{32}\.jpg$`, *repo.updateData.AvatarUrl)
	assert.Equal(t, []string{oldAvatar}, store.deletes)
}

func TestFriendLinkService_Update_RejectsInvalidStatus(t *testing.T) {
	status := uint8(3)
	repo := &fakeFriendLinkRepository{}
	svc := friendlink.NewFriendLinkService(repo, &fakeFriendLinkObjectStore{})

	_, err := svc.Update(7, dto.FriendLinkUpdateReq{
		Status: &status,
		Logo:   &dto.UploadedImageFile{Name: "logo.png", Data: friendLinkPNG(t, 120, 120)},
	})
	require.ErrorIs(t, err, friendlink.ErrFriendLinkStatusInvalid)
}

func TestFriendLinkService_Create_RejectsMissingLogo(t *testing.T) {
	seq := uint(1)
	repo := &fakeFriendLinkRepository{}
	svc := friendlink.NewFriendLinkService(repo, &fakeFriendLinkObjectStore{})

	_, err := svc.Create(dto.FriendLinkCreateReq{
		Name: "友站",
		Site: "https://friend.example.com",
		Seq:  &seq,
	})

	require.ErrorIs(t, err, friendlink.ErrFriendLinkLogoRequired)
	assert.Empty(t, repo.createGot.Name)
}

func TestFriendLinkService_Create_DeletesUploadedLogoWhenRepositoryFails(t *testing.T) {
	seq := uint(1)
	repo := &fakeFriendLinkRepository{createErr: errors.New("db down")}
	store := &fakeFriendLinkObjectStore{}
	svc := friendlink.NewFriendLinkService(repo, store)

	_, err := svc.Create(dto.FriendLinkCreateReq{
		Name: "友站",
		Site: "https://friend.example.com",
		Seq:  &seq,
		Logo: &dto.UploadedImageFile{Name: "logo.png", Data: friendLinkPNG(t, 120, 120)},
	})

	require.Error(t, err)
	require.Len(t, store.puts, 1)
	assert.Equal(t, []string{store.puts[0].key}, store.deletes)
}

func TestFriendLinkService_Create_RejectsGIFLogo(t *testing.T) {
	seq := uint(1)
	repo := &fakeFriendLinkRepository{}
	svc := friendlink.NewFriendLinkService(repo, &fakeFriendLinkObjectStore{})

	_, err := svc.Create(dto.FriendLinkCreateReq{
		Name: "友站",
		Site: "https://friend.example.com",
		Seq:  &seq,
		Logo: &dto.UploadedImageFile{Name: "logo.gif", Data: friendLinkGIF()},
	})

	require.ErrorIs(t, err, friendlink.ErrFriendLinkLogoGIF)
	assert.Empty(t, repo.createGot.Name)
}

func TestFriendLinkService_Update_RejectsMissingLogo(t *testing.T) {
	repo := &fakeFriendLinkRepository{}
	svc := friendlink.NewFriendLinkService(repo, &fakeFriendLinkObjectStore{})

	_, err := svc.Update(7, dto.FriendLinkUpdateReq{})

	require.ErrorIs(t, err, friendlink.ErrFriendLinkLogoRequired)
	assert.Zero(t, repo.updateID)
}

func friendLinkPNG(t *testing.T, width int, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: uint8((x + y) % 255), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func friendLinkGIF() []byte {
	return []byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
		0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
	}
}
