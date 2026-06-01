package guestbook_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	guestbookservice "github.com/vpt/blog-backend/internal/service/guestbook"
	"github.com/vpt/blog-backend/pkg/roles"
)

type fakeGuestbookRepo struct {
	listOwnerID  uint
	listViewerID *uint
	listPage     int
	listPageSize int
	listResp     *guestbookrepo.PageResult
	listErr      error

	createOwnerID uint
	createFromID  uint
	createContent string
	createResp    *guestbookrepo.GuestbookAggregate
	createErr     error

	toggleID     uint
	toggleUserID uint
	toggleResp   *guestbookrepo.LikeResult
	toggleErr    error

	deleteID     uint
	deleteUserID uint
	deleteForce  bool
	deleteResp   *model.Guestbook
	deleteErr    error
}

func (f *fakeGuestbookRepo) List(ownerUserID uint, viewerID *uint, page int, pageSize int) (*guestbookrepo.PageResult, error) {
	f.listOwnerID = ownerUserID
	f.listViewerID = viewerID
	f.listPage = page
	f.listPageSize = pageSize
	return f.listResp, f.listErr
}

func (f *fakeGuestbookRepo) Create(ownerUserID uint, fromUserID uint, content string) (*guestbookrepo.GuestbookAggregate, error) {
	f.createOwnerID = ownerUserID
	f.createFromID = fromUserID
	f.createContent = content
	return f.createResp, f.createErr
}

func (f *fakeGuestbookRepo) ToggleLike(id uint, userID uint) (*guestbookrepo.LikeResult, error) {
	f.toggleID = id
	f.toggleUserID = userID
	return f.toggleResp, f.toggleErr
}

func (f *fakeGuestbookRepo) Delete(id uint, userID uint, force bool) (*model.Guestbook, error) {
	f.deleteID = id
	f.deleteUserID = userID
	f.deleteForce = force
	return f.deleteResp, f.deleteErr
}

func TestGuestbookService_List_DefaultsOwnerAndPagination(t *testing.T) {
	viewerID := uint(7)
	repo := &fakeGuestbookRepo{
		listResp: &guestbookrepo.PageResult{
			Total:    0,
			Page:     1,
			PageSize: 10,
			Messages: []guestbookrepo.GuestbookAggregate{},
		},
	}
	svc := guestbookservice.NewGuestbookService(repo)

	resp, err := svc.List(dto.GuestbookListReq{Page: 0, PageSize: 99}, &viewerID)

	require.NoError(t, err)
	assert.Equal(t, uint(1), repo.listOwnerID)
	assert.Equal(t, &viewerID, repo.listViewerID)
	assert.Equal(t, 1, repo.listPage)
	assert.Equal(t, 50, repo.listPageSize)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 10, resp.PageSize)
}

func TestGuestbookService_Create_TrimsContentAndDefaultsOwner(t *testing.T) {
	now := time.Now()
	repo := &fakeGuestbookRepo{
		createResp: &guestbookrepo.GuestbookAggregate{
			Message: model.Guestbook{
				Base:        model.Base{ID: 9, CreatedAt: now, UpdatedAt: now},
				OwnerUserID: 1,
				FromUserID:  7,
				Content:     "你好",
			},
			LikeCount: 0,
			IsLiked:   false,
		},
	}
	svc := guestbookservice.NewGuestbookService(repo)

	resp, err := svc.Create(dto.GuestbookCreateReq{Content: "  你好  "}, 7)

	require.NoError(t, err)
	assert.Equal(t, uint(1), repo.createOwnerID)
	assert.Equal(t, uint(7), repo.createFromID)
	assert.Equal(t, "你好", repo.createContent)
	assert.Equal(t, uint(9), resp.ID)
	assert.Equal(t, "你好", resp.Content)
}

func TestGuestbookService_Create_RejectsBlankContent(t *testing.T) {
	svc := guestbookservice.NewGuestbookService(&fakeGuestbookRepo{})

	_, err := svc.Create(dto.GuestbookCreateReq{Content: "  "}, 7)

	require.ErrorIs(t, err, guestbookservice.ErrGuestbookContentRequired)
}

func TestGuestbookService_Delete_AllowsAdminForceDelete(t *testing.T) {
	repo := &fakeGuestbookRepo{
		deleteResp: &model.Guestbook{Base: model.Base{ID: 9}},
	}
	svc := guestbookservice.NewGuestbookService(repo)

	resp, err := svc.Delete(9, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.Equal(t, uint(9), repo.deleteID)
	assert.Equal(t, uint(7), repo.deleteUserID)
	assert.True(t, repo.deleteForce)
	assert.Equal(t, uint(9), resp.ID)
}

func TestGuestbookService_ToggleLike_MapsNotFound(t *testing.T) {
	repo := &fakeGuestbookRepo{toggleErr: guestbookrepo.ErrGuestbookNotFound}
	svc := guestbookservice.NewGuestbookService(repo)

	_, err := svc.ToggleLike(9, 7)

	require.ErrorIs(t, err, guestbookservice.ErrGuestbookNotFound)
}

func TestGuestbookService_List_MapsUnknownError(t *testing.T) {
	repo := &fakeGuestbookRepo{listErr: errors.New("db down")}
	svc := guestbookservice.NewGuestbookService(repo)

	_, err := svc.List(dto.GuestbookListReq{}, nil)

	require.EqualError(t, err, "db down")
}
