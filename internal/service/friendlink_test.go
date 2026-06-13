package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/internal/service"
)

type fakeFriendLinkRepository struct {
	listPublicOffset int
	listPublicLimit  int
	listPublicLinks  []model.FriendLink
	listPublicTotal  int64

	getPublicLink *model.FriendLink

	createGot  model.FriendLink
	createResp *model.FriendLink

	updateID   uint
	updateData repository.FriendLinkUpdateData
	updateResp *model.FriendLink
}

func (r *fakeFriendLinkRepository) ListPublic(offset, limit int) ([]model.FriendLink, int64, error) {
	r.listPublicOffset = offset
	r.listPublicLimit = limit
	return r.listPublicLinks, r.listPublicTotal, nil
}

func (r *fakeFriendLinkRepository) GetPublic(id uint) (*model.FriendLink, error) {
	return r.getPublicLink, nil
}

func (r *fakeFriendLinkRepository) ListAdmin(offset, limit int, status *uint8) ([]model.FriendLink, int64, error) {
	return nil, 0, nil
}

func (r *fakeFriendLinkRepository) Create(link model.FriendLink) (*model.FriendLink, error) {
	r.createGot = link
	if r.createResp != nil {
		return r.createResp, nil
	}
	link.ID = 9
	return &link, nil
}

func (r *fakeFriendLinkRepository) Update(id uint, data repository.FriendLinkUpdateData) (*model.FriendLink, error) {
	r.updateID = id
	r.updateData = data
	return r.updateResp, nil
}

func (r *fakeFriendLinkRepository) Delete(id uint) (*model.FriendLink, error) {
	return nil, nil
}

type fakeFriendLinkResolver struct {
	urls map[string]string
	got  []string
}

func (r *fakeFriendLinkResolver) ObjectURL(ctx context.Context, objectName string) (string, error) {
	r.got = append(r.got, objectName)
	return r.urls[objectName], nil
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
	svc := service.NewFriendLinkService(repo, resolver)

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
	svc := service.NewFriendLinkService(repo, nil)

	_, err := svc.GetPublic(3)
	require.ErrorIs(t, err, service.ErrFriendLinkNotFound)
}

func TestFriendLinkService_Create_DefaultsStatusAndTrimsFields(t *testing.T) {
	seq := uint(4)
	avatar := " friend/logo.png "
	repo := &fakeFriendLinkRepository{}
	svc := service.NewFriendLinkService(repo, nil)

	resp, err := svc.Create(dto.FriendLinkCreateReq{
		Name:      "  友站  ",
		Site:      " https://friend.example.com ",
		AvatarUrl: &avatar,
		Seq:       &seq,
	})
	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.ID)
	assert.Equal(t, "友站", repo.createGot.Name)
	assert.Equal(t, "https://friend.example.com", repo.createGot.Site)
	assert.Equal(t, "friend/logo.png", *repo.createGot.AvatarUrl)
	assert.Equal(t, uint8(1), repo.createGot.Status)
}

func TestFriendLinkService_Update_AllowsClearingOptionalFields(t *testing.T) {
	empty := " "
	name := " 新友站 "
	repo := &fakeFriendLinkRepository{
		updateResp: &model.FriendLink{
			Base:   model.Base{ID: 7},
			Name:   "新友站",
			Site:   "https://friend.example.com",
			Seq:    1,
			Status: 1,
		},
	}
	svc := service.NewFriendLinkService(repo, nil)

	resp, err := svc.Update(7, dto.FriendLinkUpdateReq{
		Name:        &name,
		Description: &empty,
		AvatarUrl:   &empty,
	})
	require.NoError(t, err)
	assert.Equal(t, uint(7), resp.ID)
	assert.Equal(t, uint(7), repo.updateID)
	require.NotNil(t, repo.updateData.Name)
	assert.Equal(t, "新友站", *repo.updateData.Name)
	assert.True(t, repo.updateData.UpdateDescription)
	assert.Nil(t, repo.updateData.Description)
	assert.True(t, repo.updateData.UpdateAvatarUrl)
	assert.Nil(t, repo.updateData.AvatarUrl)
}

func TestFriendLinkService_Update_RejectsInvalidStatus(t *testing.T) {
	status := uint8(2)
	repo := &fakeFriendLinkRepository{}
	svc := service.NewFriendLinkService(repo, nil)

	_, err := svc.Update(7, dto.FriendLinkUpdateReq{Status: &status})
	require.ErrorIs(t, err, service.ErrFriendLinkStatusInvalid)
}
