package guestbook_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	guestbookservice "github.com/vpt/blog-backend/internal/service/guestbook"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	moderationmock "github.com/vpt/blog-backend/internal/service/moderation/mock"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
)

type fakeGuestbookRepo struct {
	listOwnerID       uint
	listViewerID      *uint
	listPage          int
	listPageSize      int
	listResp          *guestbookrepo.PageResult
	listErr           error
	listAdminSearch   string
	listAdminPage     int
	listAdminPageSize int
	listAdminResp     *guestbookrepo.PageResult
	listAdminErr      error

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

func (f *fakeGuestbookRepo) ListAdmin(search string, page int, pageSize int) (*guestbookrepo.PageResult, error) {
	f.listAdminSearch = search
	f.listAdminPage = page
	f.listAdminPageSize = pageSize
	return f.listAdminResp, f.listAdminErr
}

func (f *fakeGuestbookRepo) Create(ownerUserID uint, fromUserID uint, content string) (*guestbookrepo.GuestbookAggregate, error) {
	f.createOwnerID = ownerUserID
	f.createFromID = fromUserID
	f.createContent = content
	if f.createResp == nil && f.createErr == nil {
		f.createResp = &guestbookrepo.GuestbookAggregate{
			Message: model.Guestbook{
				Base:        model.Base{ID: 9},
				OwnerUserID: ownerUserID,
				FromUserID:  fromUserID,
				Content:     content,
			},
		}
	}
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
			Messages: []guestbookrepo.GuestbookAggregate{
				{
					Message: model.Guestbook{
						Base:        model.Base{ID: 9},
						OwnerUserID: 1,
						FromUserID:  7,
						Content:     "你好",
					},
					ReplyCount: 2,
				},
			},
		},
	}
	svc := guestbookservice.NewGuestbookService(repo, nil, nil, nil)

	resp, err := svc.List(dto.GuestbookListReq{Page: 0, PageSize: 99}, &viewerID)

	require.NoError(t, err)
	assert.Equal(t, uint(1), repo.listOwnerID)
	assert.Equal(t, &viewerID, repo.listViewerID)
	assert.Equal(t, 1, repo.listPage)
	assert.Equal(t, 50, repo.listPageSize)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 10, resp.PageSize)
	require.Len(t, resp.List, 1)
	assert.Equal(t, int64(2), resp.List[0].ReplyCount)
}

func TestGuestbookServiceListProjectsModerationInOneBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	repo := &fakeGuestbookRepo{listResp: &guestbookrepo.PageResult{
		Page: 1, PageSize: 10, Messages: []guestbookrepo.GuestbookAggregate{{
			Message: model.Guestbook{Base: model.Base{ID: 9}, OwnerUserID: 1, FromUserID: 7, Content: "业务正文"},
		}},
	}}
	ref := moderationservice.SubjectRef{Type: moderationservice.SubjectGuestbook, ID: 9, RootID: 1}
	moderationSvc.EXPECT().LoadViews(gomock.Any(), []moderationservice.SubjectRef{ref}, moderationservice.Viewer{
		Role: moderationrepo.ViewerPublic,
	}).Return(map[moderationservice.SubjectKey]moderationservice.View{
		ref.Key(): {
			PublicState: moderationrepo.PublicPlaceholder, DisplayVersion: moderationrepo.DisplayNone,
			HasPendingRevision: true, CanInteract: false,
		},
	}, nil)
	svc := guestbookservice.NewGuestbookService(repo, nil, nil, nil, moderationSvc)

	resp, err := svc.List(dto.GuestbookListReq{}, nil)

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Empty(t, resp.List[0].Content)
	assert.Equal(t, "placeholder", resp.List[0].Moderation.PublicState)
	assert.True(t, resp.List[0].Moderation.HasPendingRevision)
}

func TestGuestbookService_ListAdmin_NormalizesSearchAndPagination(t *testing.T) {
	now := time.Now()
	repo := &fakeGuestbookRepo{
		listAdminResp: &guestbookrepo.PageResult{
			Total:    1,
			Page:     1,
			PageSize: 50,
			Messages: []guestbookrepo.GuestbookAggregate{
				{
					Message: model.Guestbook{
						Base:        model.Base{ID: 9, CreatedAt: now, UpdatedAt: now},
						OwnerUserID: 1,
						FromUserID:  7,
						Content:     "你好",
					},
					User:       &model.User{Base: model.Base{ID: 7}, Username: "vpt"},
					ReplyCount: 2,
					LikeCount:  3,
				},
			},
		},
	}
	svc := guestbookservice.NewGuestbookService(repo, nil, nil, nil)

	resp, err := svc.ListAdmin(dto.AdminGuestbookListReq{
		Page:     0,
		PageSize: 99,
		Search:   "  你好  ",
	})

	require.NoError(t, err)
	assert.Equal(t, "你好", repo.listAdminSearch)
	assert.Equal(t, 1, repo.listAdminPage)
	assert.Equal(t, 50, repo.listAdminPageSize)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "你好", resp.List[0].Content)
	assert.Equal(t, int64(2), resp.List[0].ReplyCount)
	assert.Equal(t, int64(3), resp.List[0].LikeCount)
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
	svc := guestbookservice.NewGuestbookService(repo, nil, nil, nil)

	resp, err := svc.Create(dto.GuestbookCreateReq{Content: "  你好  "}, 7)

	require.NoError(t, err)
	assert.Equal(t, uint(1), repo.createOwnerID)
	assert.Equal(t, uint(7), repo.createFromID)
	assert.Equal(t, "你好", repo.createContent)
	assert.Equal(t, uint(9), resp.ID)
	assert.Equal(t, "你好", resp.Content)
}

func TestGuestbookService_Create_NormalizesTempCommentImagesBeforeCreate(t *testing.T) {
	store := newGuestbookAssetStore()
	store.keys["temp/comments/7/images/hello.jpg"] = true
	store.keyMap["https://cdn.example.com/blog/temp/comments/7/images/hello.jpg?a=1"] = "temp/comments/7/images/hello.jpg"
	repo := &fakeGuestbookRepo{}
	svc := guestbookservice.NewGuestbookService(repo, store, nil, nil)

	resp, err := svc.Create(dto.GuestbookCreateReq{
		OwnerUserID: 1,
		Content:     ` 留言图 <img src="https://cdn.example.com/blog/temp/comments/7/images/hello.jpg?a=1"> `,
	}, 7)

	require.NoError(t, err)
	assert.Equal(t, `留言图 <img src="comments/guestbook/1/images/hello.jpg">`, repo.createContent)
	assert.Equal(t, []guestbookAssetCopy{{
		source: "temp/comments/7/images/hello.jpg",
		target: "comments/guestbook/1/images/hello.jpg",
	}}, store.copies)
	require.NotNil(t, resp)
	assert.Equal(t, `留言图 <img src="https://cdn.example.com/blog/comments/guestbook/1/images/hello.jpg">`, resp.Content)
}

func TestGuestbookService_Create_CleansCopiedCommentImagesWhenRepositoryFails(t *testing.T) {
	store := newGuestbookAssetStore()
	store.keys["temp/comments/7/images/hello.jpg"] = true
	store.keyMap["https://cdn.example.com/blog/temp/comments/7/images/hello.jpg"] = "temp/comments/7/images/hello.jpg"
	repo := &fakeGuestbookRepo{createErr: errors.New("db down")}
	svc := guestbookservice.NewGuestbookService(repo, store, nil, nil)

	_, err := svc.Create(dto.GuestbookCreateReq{
		OwnerUserID: 1,
		Content:     "![hello](https://cdn.example.com/blog/temp/comments/7/images/hello.jpg)",
	}, 7)

	require.EqualError(t, err, "db down")
	assert.Equal(t, []string{"comments/guestbook/1/images/hello.jpg"}, store.deleted)
}

func TestGuestbookService_Create_RejectsBlankContent(t *testing.T) {
	svc := guestbookservice.NewGuestbookService(&fakeGuestbookRepo{}, nil, nil, nil)

	_, err := svc.Create(dto.GuestbookCreateReq{Content: "  "}, 7)

	require.ErrorIs(t, err, guestbookservice.ErrGuestbookContentRequired)
}

func TestGuestbookService_Delete_AllowsAdminForceDelete(t *testing.T) {
	repo := &fakeGuestbookRepo{
		deleteResp: &model.Guestbook{Base: model.Base{ID: 9}},
	}
	svc := guestbookservice.NewGuestbookService(repo, nil, nil, nil)

	resp, err := svc.Delete(9, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.Equal(t, uint(9), repo.deleteID)
	assert.Equal(t, uint(7), repo.deleteUserID)
	assert.True(t, repo.deleteForce)
	assert.Equal(t, uint(9), resp.ID)
}

func TestGuestbookService_ToggleLike_MapsNotFound(t *testing.T) {
	repo := &fakeGuestbookRepo{toggleErr: guestbookrepo.ErrGuestbookNotFound}
	svc := guestbookservice.NewGuestbookService(repo, nil, nil, nil)

	_, err := svc.ToggleLike(9, 7)

	require.ErrorIs(t, err, guestbookservice.ErrGuestbookNotFound)
}

func TestGuestbookService_List_MapsUnknownError(t *testing.T) {
	repo := &fakeGuestbookRepo{listErr: errors.New("db down")}
	svc := guestbookservice.NewGuestbookService(repo, nil, nil, nil)

	_, err := svc.List(dto.GuestbookListReq{}, nil)

	require.EqualError(t, err, "db down")
}

type guestbookAssetStore struct {
	keys    map[string]bool
	keyMap  map[string]string
	copies  []guestbookAssetCopy
	deleted []string
}

type guestbookAssetCopy struct {
	source string
	target string
}

func newGuestbookAssetStore() *guestbookAssetStore {
	return &guestbookAssetStore{
		keys:   map[string]bool{},
		keyMap: map[string]string{},
	}
}

func (s *guestbookAssetStore) ObjectURL(_ context.Context, objectName string) (string, error) {
	return "https://cdn.example.com/blog/" + objectName, nil
}

func (s *guestbookAssetStore) ObjectExists(_ context.Context, objectName string) (bool, error) {
	return s.keys[objectName], nil
}

func (s *guestbookAssetStore) PutObject(context.Context, string, []byte, string) error {
	return nil
}

func (s *guestbookAssetStore) DeleteObject(_ context.Context, objectName string) error {
	s.deleted = append(s.deleted, objectName)
	delete(s.keys, objectName)
	return nil
}

func (s *guestbookAssetStore) MoveObject(context.Context, string, string) error {
	return nil
}

func (s *guestbookAssetStore) CopyObject(_ context.Context, sourceName string, targetName string) error {
	s.copies = append(s.copies, guestbookAssetCopy{source: sourceName, target: targetName})
	s.keys[targetName] = true
	return nil
}

func (s *guestbookAssetStore) ObjectKey(value string) (string, error) {
	if key, ok := s.keyMap[value]; ok {
		return key, nil
	}
	value = strings.TrimLeft(strings.TrimSpace(value), "/")
	if strings.HasPrefix(value, "temp/comments/") || strings.HasPrefix(value, "comments/") {
		return value, nil
	}
	return "", storage.ErrExternalObjectURL
}

// recordingPublisher 记录发布事件，用于断言是否发布通知。
type recordingPublisher struct {
	events []notificationservice.PublishEvent
}

func (p *recordingPublisher) Publish(_ context.Context, e notificationservice.PublishEvent) (*model.NotificationEvent, error) {
	p.events = append(p.events, e)
	return &model.NotificationEvent{}, nil
}

// 自己在自己留言板留言不产生通知事件。
func TestGuestbookService_Create_SelfMessageDoesNotPublish(t *testing.T) {
	repo := &fakeGuestbookRepo{
		createResp: &guestbookrepo.GuestbookAggregate{
			Message: model.Guestbook{Base: model.Base{ID: 9}, OwnerUserID: 7, FromUserID: 7, Content: "自言自语"},
		},
	}
	pub := &recordingPublisher{}
	svc := guestbookservice.NewGuestbookService(repo, nil, pub, nil)

	_, err := svc.Create(dto.GuestbookCreateReq{OwnerUserID: 7, Content: "自言自语"}, 7)

	require.NoError(t, err)
	assert.Empty(t, pub.events)
}

// 板主与留言者不同时正常发布通知事件。
func TestGuestbookService_Create_PublishesForOtherOwner(t *testing.T) {
	repo := &fakeGuestbookRepo{
		createResp: &guestbookrepo.GuestbookAggregate{
			Message: model.Guestbook{Base: model.Base{ID: 9}, OwnerUserID: 1, FromUserID: 7, Content: "你好"},
		},
	}
	pub := &recordingPublisher{}
	svc := guestbookservice.NewGuestbookService(repo, nil, pub, nil)

	_, err := svc.Create(dto.GuestbookCreateReq{OwnerUserID: 1, Content: "你好"}, 7)

	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeGuestbookCreated, pub.events[0].Type)
}

// 自己点赞自己的留言不产生通知事件。
func TestGuestbookService_ToggleLike_SelfLikeDoesNotPublish(t *testing.T) {
	repo := &fakeGuestbookRepo{
		toggleResp: &guestbookrepo.LikeResult{ID: 9, IsLiked: true, LikeCount: 2, OwnerUserID: 7},
	}
	pub := &recordingPublisher{}
	svc := guestbookservice.NewGuestbookService(repo, nil, pub, nil)

	_, err := svc.ToggleLike(9, 7)

	require.NoError(t, err)
	assert.Empty(t, pub.events)
}

// 点赞别人的留言正常发布通知事件。
func TestGuestbookService_ToggleLike_PublishesForOtherOwner(t *testing.T) {
	repo := &fakeGuestbookRepo{
		toggleResp: &guestbookrepo.LikeResult{ID: 9, IsLiked: true, LikeCount: 2, OwnerUserID: 1, Content: "留言正文"},
	}
	pub := &recordingPublisher{}
	svc := guestbookservice.NewGuestbookService(repo, nil, pub, nil)

	_, err := svc.ToggleLike(9, 7)

	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeGuestbookLiked, pub.events[0].Type)
	require.NotNil(t, pub.events[0].Metadata)
	assert.Contains(t, *pub.events[0].Metadata, `"source_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"root_snapshot"`)
	assert.Contains(t, *pub.events[0].Metadata, `"excerpt":"留言正文"`)
}
