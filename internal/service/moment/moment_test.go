package moment_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
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
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	moderationmock "github.com/vpt/blog-backend/internal/service/moderation/mock"
	momentservice "github.com/vpt/blog-backend/internal/service/moment"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	"github.com/vpt/blog-backend/pkg/roles"
	"go.uber.org/mock/gomock"
)

type fakeMomentRepo struct {
	listFilter      momentrepo.ListFilter
	listViewerID    *uint
	listResp        *momentrepo.PageResult
	listErr         error
	listAdminFilter momentrepo.AdminListFilter
	listAdminResp   *momentrepo.PageResult
	listAdminErr    error

	feedFilter   momentrepo.FeedFilter
	feedViewerID *uint
	feedResp     *momentrepo.PageResult
	feedErr      error

	saveData momentrepo.SaveData
	saveResp *momentrepo.MomentAggregate
	saveErr  error
	removed  []string

	deleteID       uint
	deleteOperator uint
	deleteForce    bool
	deleteResp     *model.Moment
	deleteMedia    []model.Media
	deleteErr      error

	topID       uint
	topOperator uint
	topForce    bool
	topResp     *model.Moment
	topErr      error

	readID   uint
	readResp *model.Moment
	readErr  error

	likeID     uint
	likeUserID uint
	likeResp   *momentrepo.MomentAggregate
	likeErr    error

	isLikedResp   bool
	likeCountResp int64
	isLikedErr    error

	countUserID uint
	countResp   int64
	countErr    error
}

func (f *fakeMomentRepo) List(filter momentrepo.ListFilter, viewerID *uint) (*momentrepo.PageResult, error) {
	f.listFilter = filter
	f.listViewerID = viewerID
	return f.listResp, f.listErr
}

func (f *fakeMomentRepo) ListAdmin(filter momentrepo.AdminListFilter) (*momentrepo.PageResult, error) {
	f.listAdminFilter = filter
	return f.listAdminResp, f.listAdminErr
}

func (f *fakeMomentRepo) ListFeed(filter momentrepo.FeedFilter, viewerID *uint) (*momentrepo.PageResult, error) {
	f.feedFilter = filter
	f.feedViewerID = viewerID
	return f.feedResp, f.feedErr
}

func (f *fakeMomentRepo) FindPublicDetail(uint, *uint) (*momentrepo.MomentAggregate, error) {
	return nil, momentrepo.ErrMomentNotFound
}

func (f *fakeMomentRepo) Save(data momentrepo.SaveData) (*momentrepo.MomentAggregate, error) {
	if data.Moment.ID == 0 {
		data.Moment.ID = uint(9)
		if f.saveResp != nil && f.saveResp.Moment.ID > 0 {
			data.Moment.ID = f.saveResp.Moment.ID
		}
	}
	if data.PrepareImages != nil {
		images, err := data.PrepareImages(data.Moment)
		if err != nil {
			return nil, err
		}
		data.Images = images
	}
	if data.RemovedURLs != nil {
		*data.RemovedURLs = append(*data.RemovedURLs, f.removed...)
	}
	f.saveData = data
	return f.saveResp, f.saveErr
}

func (f *fakeMomentRepo) Delete(id uint, operatorID uint, force bool) (*model.Moment, []model.Media, error) {
	f.deleteID = id
	f.deleteOperator = operatorID
	f.deleteForce = force
	return f.deleteResp, f.deleteMedia, f.deleteErr
}

func (f *fakeMomentRepo) SetTop(id uint, operatorID uint, force bool) (*model.Moment, error) {
	f.topID = id
	f.topOperator = operatorID
	f.topForce = force
	return f.topResp, f.topErr
}

func (f *fakeMomentRepo) RemoveTop(id uint, operatorID uint, force bool) (*model.Moment, error) {
	f.topID = id
	f.topOperator = operatorID
	f.topForce = force
	return f.topResp, f.topErr
}

func (f *fakeMomentRepo) IncrementReadCount(id uint) (*model.Moment, error) {
	f.readID = id
	return f.readResp, f.readErr
}

func (f *fakeMomentRepo) IsLiked(id uint, userID uint) (bool, int64, error) {
	f.likeID = id
	f.likeUserID = userID
	return f.isLikedResp, f.likeCountResp, f.isLikedErr
}

func (f *fakeMomentRepo) ToggleLike(id uint, userID uint) (*momentrepo.MomentAggregate, bool, error) {
	f.likeID = id
	f.likeUserID = userID
	return f.likeResp, true, f.likeErr
}

func (f *fakeMomentRepo) CountPublicByUser(userID uint) (int64, error) {
	f.countUserID = userID
	return f.countResp, f.countErr
}

type fakeURLResolver struct {
	objects []string
}

func (r *fakeURLResolver) ObjectURL(_ context.Context, objectName string) (string, error) {
	r.objects = append(r.objects, objectName)
	return "https://cdn.example.com/" + objectName, nil
}

type fakeMomentObjectStore struct {
	fakeURLResolver
	exists       map[string]bool
	existsErr    error
	putKeys      []string
	putErrOnKey  string
	putErrOnCall int
	deleteKeys   []string
	deleteErr    error
	uploadedData map[string][]byte
	uploadedType map[string]string
}

func (s *fakeMomentObjectStore) ObjectExists(_ context.Context, objectName string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.exists[objectName], nil
}

func (s *fakeMomentObjectStore) PutObject(_ context.Context, objectName string, data []byte, contentType string) error {
	s.putKeys = append(s.putKeys, objectName)
	if s.putErrOnCall > 0 && len(s.putKeys) == s.putErrOnCall {
		return errors.New("put failed")
	}
	if s.putErrOnKey == objectName {
		return errors.New("put failed")
	}
	if s.uploadedData == nil {
		s.uploadedData = map[string][]byte{}
	}
	if s.uploadedType == nil {
		s.uploadedType = map[string]string{}
	}
	s.uploadedData[objectName] = append([]byte(nil), data...)
	s.uploadedType[objectName] = contentType
	return nil
}

func (s *fakeMomentObjectStore) DeleteObject(_ context.Context, objectName string) error {
	s.deleteKeys = append(s.deleteKeys, objectName)
	return s.deleteErr
}

func (s *fakeMomentObjectStore) MoveObject(context.Context, string, string) error {
	return nil
}

func (s *fakeMomentObjectStore) CopyObject(context.Context, string, string) error {
	return nil
}

func (s *fakeMomentObjectStore) ObjectKey(value string) (string, error) {
	return value, nil
}

func TestMomentService_CountByUser_ReturnsTotal(t *testing.T) {
	repo := &fakeMomentRepo{countResp: 8}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil, nil, nil, nil)

	resp, err := svc.CountByUser(7)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(8), resp.Count)
	assert.Equal(t, uint(7), repo.countUserID)
}

func TestMomentService_List_NormalizesPaginationAndResolvesImages(t *testing.T) {
	now := time.Now()
	viewerID := uint(7)
	repo := &fakeMomentRepo{
		listResp: &momentrepo.PageResult{
			Total:    1,
			Page:     1,
			PageSize: 50,
			Moments: []momentrepo.MomentAggregate{{
				Moment: model.Moment{Base: model.Base{ID: 9, CreatedAt: now, UpdatedAt: now}, UserID: 1, Content: "风", Status: 1, CommentStatus: 1},
				Images: []model.Media{{Base: model.Base{ID: 3}, MomentID: 9, URL: "moments/cat.jpg", Name: "cat.jpg"}},
			}},
		},
	}
	resolver := &fakeURLResolver{}
	svc := momentservice.NewMomentService(repo, resolver, nil, nil, nil, nil)

	resp, err := svc.List(dto.MomentListReq{Page: 0, PageSize: 99}, &viewerID)

	require.NoError(t, err)
	assert.Equal(t, 1, repo.listFilter.Page)
	assert.Equal(t, 50, repo.listFilter.PageSize)
	assert.Equal(t, &viewerID, repo.listViewerID)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "https://cdn.example.com/moments/cat.jpg", resp.List[0].Images[0].AccessURL)
	assert.Equal(t, []string{"moments/cat.jpg"}, resolver.objects)
}

func TestMomentServiceListProjectsModerationInOneBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	repo := &fakeMomentRepo{listResp: &momentrepo.PageResult{
		Page: 1, PageSize: 10, Moments: []momentrepo.MomentAggregate{{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "旧正文", Status: 1},
		}},
	}}
	ref := moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: 9}
	risk := moderationrepo.RiskMedium
	status := moderationrepo.ReviewPending
	pending := "新中风险正文"
	moderationSvc.EXPECT().LoadViews(gomock.Any(), []moderationservice.SubjectRef{ref}, moderationservice.Viewer{
		Role: moderationrepo.ViewerAuthor, UserID: 7,
	}).Return(map[moderationservice.SubjectKey]moderationservice.View{
		ref.Key(): {
			PublicState: moderationrepo.PublicVisible, DisplayVersion: moderationrepo.DisplayLastApproved,
			VisibleContent: "最后通过正文", HasPendingRevision: true,
			PendingContent: &pending, PendingRiskLevel: &risk, PendingReviewStatus: &status,
			CanInteract:   false,
			VisibleImages: []moderationrepo.ImageView{{Seq: 1, DisplayObjectKey: "moderation/previews/a.jpg"}},
		},
	}, nil)
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil, nil, nil, moderationSvc)
	viewerID := uint(7)

	resp, err := svc.List(dto.MomentListReq{}, &viewerID)

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "最后通过正文", resp.List[0].Content)
	assert.Equal(t, "last_approved", resp.List[0].Moderation.DisplayVersion)
	assert.Equal(t, &pending, resp.List[0].Moderation.PendingContent)
	require.Len(t, resp.List[0].Images, 1)
	assert.Equal(t, "moderation/previews/a.jpg", resp.List[0].Images[0].URL)
	assert.Equal(t, "https://cdn.example.com/moderation/previews/a.jpg", resp.List[0].Images[0].AccessURL)
}

func TestMomentServiceSaveUsesModerationBeforeBusinessRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	repo := &fakeMomentRepo{}
	moderationSvc.EXPECT().Submit(gomock.Any(), moderationservice.SubmitCommand{
		ActorID: 7, AuthorID: 7, Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectMoment},
		Content: "碎语", IdempotencyKey: "moment-key",
		MomentOptions: &moderationservice.MomentOptions{Status: 1, CommentStatus: 1},
	}).Return(moderationservice.SubmitResult{
		Subject:   moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: 9},
		AuthorID:  7,
		RiskLevel: moderationservice.RiskLow, Action: moderationservice.ActionPostReview,
		PublicState: moderationservice.PublicVisible, ReviewStatus: moderationservice.ReviewPending,
		Content: "碎语", HasPendingRevision: true,
	}, nil)
	svc := momentservice.NewMomentService(repo, nil, nil, nil, nil, moderationSvc)

	resp, err := svc.Save(dto.MomentSaveReq{
		Content: "碎语", Status: 1, CommentStatus: 1, IdempotencyKey: "moment-key",
	}, 7, nil)

	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.ID)
	assert.Zero(t, repo.saveData.Moment.UserID, "业务仓储不得重复保存碎语")
	assert.True(t, resp.Moderation.HasPendingRevision)
}

func TestMomentModeratedSaveUploadsFilesAndPassesOrderedObjectKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	fileData := append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 32)...)
	fileKey := "moments/7/moderation/" + md5Hex(fileData) + ".gif"
	store := &fakeMomentObjectStore{exists: map[string]bool{"moments/7/old.jpg": true}}
	moderationSvc.EXPECT().Submit(gomock.Any(), moderationservice.SubmitCommand{
		ActorID: 7, AuthorID: 7, Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectMoment},
		Content: "碎语", ImageKeys: []string{"moments/7/old.jpg", fileKey}, IdempotencyKey: "moment-images",
		MomentOptions: &moderationservice.MomentOptions{Status: 1, CommentStatus: 1},
	}).Return(moderationservice.SubmitResult{
		Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: 9}, AuthorID: 7,
		Images: []moderationrepo.ImageView{{Seq: 1, DisplayObjectKey: "moderation/previews/a.jpg"}},
	}, nil)
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil, nil, nil, moderationSvc)

	resp, err := svc.Save(dto.MomentSaveReq{
		Content: "碎语", Status: 1, CommentStatus: 1, IdempotencyKey: "moment-images",
		ImageURLs:  []string{"moments/7/old.jpg"},
		ImageFiles: []dto.MomentImageFileReq{{Name: "cat.gif", ContentType: "image/gif", Data: fileData}},
	}, 7, nil)

	require.NoError(t, err)
	assert.Contains(t, store.putKeys, fileKey)
	require.Len(t, resp.Images, 1)
	assert.Equal(t, "moderation/previews/a.jpg", resp.Images[0].URL)
}

func TestMomentModeratedSavePreservesAdminManagedAuthor(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	authorID := uint(99)
	moderationSvc.EXPECT().Submit(gomock.Any(), moderationservice.SubmitCommand{
		ActorID: 7, AuthorID: 99, IsAdmin: true,
		Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectMoment},
		Content: "代管碎语", IdempotencyKey: "managed-author",
		MomentOptions: &moderationservice.MomentOptions{Status: 1, CommentStatus: 1},
	}).Return(moderationservice.SubmitResult{
		Subject:   moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: 9},
		AuthorID:  99,
		RiskLevel: moderationservice.RiskLow, PublicState: moderationservice.PublicVisible,
		ReviewStatus: moderationservice.ReviewPending, Content: "代管碎语", HasPendingRevision: true,
	}, nil)
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, nil, nil, nil, nil, moderationSvc)

	resp, err := svc.Save(dto.MomentSaveReq{
		UserID: &authorID, Content: "代管碎语", Status: 1, CommentStatus: 1, IdempotencyKey: "managed-author",
	}, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.Equal(t, uint(99), resp.UserID)
}

func TestMomentServiceEditUsesModeration(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	id := uint(9)
	moderationSvc.EXPECT().Edit(gomock.Any(), moderationservice.EditCommand{
		ActorID: 7, Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: 9},
		Content: "编辑碎语", IdempotencyKey: "moment-edit",
		MomentOptions: &moderationservice.MomentOptions{},
	}).Return(moderationservice.SubmitResult{
		Subject:   moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: 9},
		AuthorID:  7,
		RiskLevel: moderationservice.RiskMedium, PublicState: moderationservice.PublicVisible,
		ReviewStatus: moderationservice.ReviewPending, Content: "旧碎语", HasPendingRevision: true,
	}, nil)
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, nil, nil, nil, nil, moderationSvc)

	resp, err := svc.Save(dto.MomentSaveReq{ID: &id, Content: "编辑碎语", IdempotencyKey: "moment-edit"}, 7, nil)
	require.NoError(t, err)
	assert.Equal(t, "旧碎语", resp.Content)
}

func TestMomentAdminEditKeepsOriginalAuthorInResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	id := uint(9)
	moderationSvc.EXPECT().Edit(gomock.Any(), gomock.Any()).Return(moderationservice.SubmitResult{
		Subject:  moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: 9},
		AuthorID: 99, RiskLevel: moderationservice.RiskLow,
		PublicState: moderationservice.PublicVisible, ReviewStatus: moderationservice.ReviewPending,
		Content: "编辑后碎语", HasPendingRevision: true,
	}, nil)
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, nil, nil, nil, nil, moderationSvc)

	resp, err := svc.Save(dto.MomentSaveReq{
		ID: &id, Content: "编辑后碎语", IdempotencyKey: "admin-edit",
	}, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.Equal(t, uint(99), resp.UserID)
}

func TestMomentServiceToggleLikeGuardsPendingContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	moderationSvc.EXPECT().AssertCanInteract(gomock.Any(), moderationservice.SubjectRef{
		Type: moderationservice.SubjectMoment, ID: 9,
	}).Return(moderationservice.ErrInteractionNotAllowed)
	repo := &fakeMomentRepo{}
	svc := momentservice.NewMomentService(repo, nil, nil, nil, nil, moderationSvc)

	_, err := svc.ToggleLike(9, 7)

	assert.ErrorIs(t, err, moderationservice.ErrInteractionNotAllowed)
	assert.Zero(t, repo.likeID)
}

func TestMomentServiceDeletesThroughModeration(t *testing.T) {
	ctrl := gomock.NewController(t)
	moderationSvc := moderationmock.NewMockService(ctrl)
	moderationSvc.EXPECT().Delete(gomock.Any(), moderationservice.DeleteCommand{
		ActorID: 7, Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: 9},
	}).Return(nil)
	repo := &fakeMomentRepo{}
	svc := momentservice.NewMomentService(repo, nil, nil, nil, nil, moderationSvc)

	resp, err := svc.Delete(9, 7, nil)

	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.ID)
	assert.Zero(t, repo.deleteID)
}

func TestMomentService_List_ForwardsRandomAndExcludeIDs(t *testing.T) {
	repo := &fakeMomentRepo{
		listResp: &momentrepo.PageResult{
			Total:    0,
			Page:     1,
			PageSize: 3,
			Moments:  nil,
		},
	}
	svc := momentservice.NewMomentService(repo, nil, nil, nil, nil, nil)

	_, err := svc.List(dto.MomentListReq{
		Random:     true,
		ExcludeIDs: []uint{50, 49, 48},
		PageSize:   3,
	}, nil)

	require.NoError(t, err)
	assert.True(t, repo.listFilter.Random)
	assert.Equal(t, []uint{50, 49, 48}, repo.listFilter.ExcludeIDs)
	assert.Equal(t, 3, repo.listFilter.PageSize)
}

func TestMomentService_ListAdmin_NormalizesFiltersAndMapsItems(t *testing.T) {
	now := time.Now()
	repo := &fakeMomentRepo{
		listAdminResp: &momentrepo.PageResult{
			Total:    1,
			Page:     1,
			PageSize: 50,
			Moments: []momentrepo.MomentAggregate{{
				Moment: model.Moment{
					Base:          model.Base{ID: 9, CreatedAt: now, UpdatedAt: now},
					UserID:        7,
					Content:       "风",
					Status:        0,
					CommentStatus: 1,
				},
				User:         &model.User{Base: model.Base{ID: 7}, Username: "vpt"},
				LikeCount:    2,
				CommentCount: 3,
			}},
		},
	}
	svc := momentservice.NewMomentService(repo, nil, nil, nil, nil, nil)

	resp, err := svc.ListAdmin(dto.AdminMomentListReq{
		Page:     0,
		PageSize: 99,
		Status:   "hidden",
		Search:   "  风  ",
	})

	require.NoError(t, err)
	assert.Equal(t, 1, repo.listAdminFilter.Page)
	assert.Equal(t, 50, repo.listAdminFilter.PageSize)
	require.NotNil(t, repo.listAdminFilter.Status)
	assert.Equal(t, uint8(0), *repo.listAdminFilter.Status)
	assert.Equal(t, "风", repo.listAdminFilter.Search)
	require.Len(t, resp.List, 1)
	assert.Equal(t, uint8(0), resp.List[0].Status)
	assert.Equal(t, int64(2), resp.List[0].LikeCount)
	assert.Equal(t, int64(3), resp.List[0].CommentCount)
}

func TestMomentService_FeedList_NormalizesFilter(t *testing.T) {
	repo := &fakeMomentRepo{
		feedResp: &momentrepo.PageResult{
			Total:    0,
			Page:     1,
			PageSize: 10,
			Moments:  nil,
		},
	}
	svc := momentservice.NewMomentService(repo, nil, nil, nil, nil, nil)

	_, err := svc.FeedList(dto.MomentFeedReq{Scope: "owner", Sort: "hot", Page: 0, PageSize: 99}, nil)

	require.NoError(t, err)
	assert.Equal(t, momentrepo.FeedScopeOwner, repo.feedFilter.Scope)
	assert.Equal(t, momentrepo.FeedSortHot, repo.feedFilter.Sort)
	assert.Equal(t, uint(1), repo.feedFilter.OwnerUserID)
	assert.Equal(t, 1, repo.feedFilter.Page)
	assert.Equal(t, 50, repo.feedFilter.PageSize)
}

func TestMomentService_Save_TrimsContentAndUsesCurrentUserForNormalRole(t *testing.T) {
	now := time.Now()
	requestUserID := uint(99)
	store := &fakeMomentObjectStore{
		exists: map[string]bool{"moments/old.jpg": true},
	}
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9, CreatedAt: now, UpdatedAt: now}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	resp, err := svc.Save(dto.MomentSaveReq{
		UserID:        &requestUserID,
		Content:       "  风  ",
		Status:        1,
		CommentStatus: 1,
		ImageURLs:     []string{"https://cdn.example.com/blog/moments/old.jpg?sign=1"},
		ImageOrder:    []string{"file:0", "url:0"},
		ImageFiles:    []dto.MomentImageFileReq{{Name: "cat.png", ContentType: "image/png", Data: smallPNG(t)}},
	}, 7, nil)

	require.NoError(t, err)
	assert.Equal(t, uint(7), repo.saveData.Moment.UserID)
	assert.Equal(t, "风", repo.saveData.Moment.Content)
	assert.False(t, repo.saveData.Force)
	assert.Equal(t, uint(7), repo.saveData.OperatorID)
	require.Len(t, repo.saveData.Images, 2)
	assert.Equal(t, "cat.png", repo.saveData.Images[0].Name)
	assert.Equal(t, "png", repo.saveData.Images[0].FileType)
	assert.Equal(t, uint(1), repo.saveData.Images[0].Seq)
	uploaded := store.uploadedData[repo.saveData.Images[0].URL]
	assert.Equal(t, "moments/7/9/"+md5Hex(uploaded)+".png", repo.saveData.Images[0].URL)
	assert.Equal(t, uint(len(uploaded)), repo.saveData.Images[0].Size)
	assert.Equal(t, "moments/old.jpg", repo.saveData.Images[1].URL)
	assert.Equal(t, uint(2), repo.saveData.Images[1].Seq)
	assert.Equal(t, uint(9), resp.ID)
}

func TestMomentService_Save_RejectsIncompleteImageOrder(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{"moments/old.jpg": true}}
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageURLs:     []string{"moments/old.jpg"},
		ImageOrder:    []string{"file:0"},
		ImageFiles:    []dto.MomentImageFileReq{{Name: "cat.png", ContentType: "image/png", Data: smallPNG(t)}},
	}, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentImageInvalid)
}

func TestMomentService_Save_RejectsMissingExistingImage(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{"moments/missing.jpg": false}}
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageURLs:     []string{"https://cdn.example.com/blog/moments/missing.jpg"},
	}, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentImageNotFound)
	assert.Empty(t, store.putKeys)
}

func TestMomentService_Save_RejectsMoreThanNineImages(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil, nil, nil, nil)

	files := make([]dto.MomentImageFileReq, 10)
	for i := range files {
		files[i] = dto.MomentImageFileReq{Name: "cat.png", ContentType: "image/png", Data: smallPNG(t)}
	}

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles:    files,
	}, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentImageInvalid)
	assert.Empty(t, store.putKeys)
}

func TestMomentService_Save_RejectsImageLargerThanOneMB(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles: []dto.MomentImageFileReq{{
			Name:        "big.jpg",
			ContentType: "image/jpeg",
			Data:        bytes.Repeat([]byte{1}, 1024*1024+1),
		}},
	}, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentImageTooLarge)
	assert.Empty(t, store.putKeys)
}

func TestMomentService_Save_ReturnsReadableMessageForBrokenImage(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles: []dto.MomentImageFileReq{{
			Name:        "broken.png",
			ContentType: "image/png",
			Data:        []byte("not a real image"),
		}},
	}, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentImageInvalid)
	assert.EqualError(t, err, "图片无法读取，请确认文件未损坏，并尝试换一张 JPG、PNG、WebP 或 300KB 以内的 GIF")
	assert.Empty(t, store.putKeys)
}

func TestMomentService_Save_ReturnsReadableMessageForOversizedGif(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles: []dto.MomentImageFileReq{{
			Name:        "motion.gif",
			ContentType: "image/gif",
			Data:        append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 300*1024-5)...),
		}},
	}, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentImageInvalid)
	assert.EqualError(t, err, "GIF 图片过大，暂不支持压缩该格式，请上传 300KB 以内的 GIF。")
	assert.Empty(t, store.putKeys)
}

func TestMomentService_Save_CompressesLargeImageToFiveHundredKB(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles: []dto.MomentImageFileReq{{
			Name:        "large.png",
			ContentType: "image/png",
			Data:        noisyPNG(t, 900, 900),
		}},
	}, 7, nil)

	require.NoError(t, err)
	require.Len(t, repo.saveData.Images, 1)
	uploaded := store.uploadedData[repo.saveData.Images[0].URL]
	require.NotEmpty(t, uploaded)
	assert.LessOrEqual(t, len(uploaded), 500*1024)
	assert.Equal(t, uint(len(uploaded)), repo.saveData.Images[0].Size)
}

func TestMomentService_Save_KeepsSmallGifOriginal(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	gif := append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 128)...)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles: []dto.MomentImageFileReq{{
			Name:        "motion.gif",
			ContentType: "image/gif",
			Data:        gif,
		}},
	}, 7, nil)

	require.NoError(t, err)
	require.Len(t, repo.saveData.Images, 1)
	image := repo.saveData.Images[0]
	assert.Equal(t, "gif", image.FileType)
	assert.Equal(t, "moments/7/9/"+md5Hex(gif)+".gif", image.URL)
	assert.Equal(t, uint(len(gif)), image.Size)
	assert.Equal(t, gif, store.uploadedData[image.URL])
	assert.Equal(t, "image/gif", store.uploadedType[image.URL])
}

func TestMomentService_Save_AcceptsWebP(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles: []dto.MomentImageFileReq{{
			Name:        "photo.webp",
			ContentType: "image/webp",
			Data:        smallWebP(t),
		}},
	}, 7, nil)

	require.NoError(t, err)
	require.Len(t, repo.saveData.Images, 1)
	image := repo.saveData.Images[0]
	assert.Equal(t, "jpg", image.FileType)
	assert.Contains(t, image.URL, "moments/7/9/")
	assert.Contains(t, image.URL, ".jpg")
	assert.Equal(t, "image/jpeg", store.uploadedType[image.URL])
	assert.NotEmpty(t, store.uploadedData[image.URL])
}

func TestMomentService_Save_DeletesRemovedOldImagesAfterSuccessfulSave(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{"moments/7/9/keep.jpg": true}}
	repo := &fakeMomentRepo{
		removed: []string{"https://cdn.example.com/blog/moments/7/9/remove.jpg?sign=1"},
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		ID:            ptrUint(9),
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageURLs:     []string{"moments/7/9/keep.jpg"},
	}, 7, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"moments/7/9/remove.jpg"}, store.deleteKeys)
}

func TestMomentService_Save_DeletesUploadedImagesWhenRepositoryFails(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	repo := &fakeMomentRepo{saveErr: errors.New("db down")}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles:    []dto.MomentImageFileReq{{Name: "cat.png", ContentType: "image/png", Data: smallPNG(t)}},
	}, 7, nil)

	require.EqualError(t, err, "db down")
	require.Len(t, store.putKeys, 1)
	assert.Equal(t, store.putKeys, store.deleteKeys)
}

func ptrUint(value uint) *uint {
	return &value
}

func TestMomentService_Save_DeletesUploadedImagesWhenLaterUploadFails(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}, putErrOnCall: 2}
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{
		Content:       "风",
		Status:        1,
		CommentStatus: 1,
		ImageFiles: []dto.MomentImageFileReq{
			{Name: "cat.png", ContentType: "image/png", Data: smallPNG(t)},
			{Name: "dog.png", ContentType: "image/png", Data: noisyPNG(t, 9, 9)},
		},
	}, 7, nil)

	require.EqualError(t, err, "put failed")
	require.Len(t, store.putKeys, 2)
	assert.Equal(t, []string{store.putKeys[0]}, store.deleteKeys)
}

func TestMomentService_Delete_RemovesMediaFilesFromGarage(t *testing.T) {
	store := &fakeMomentObjectStore{}
	repo := &fakeMomentRepo{
		deleteResp: &model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风"},
		deleteMedia: []model.Media{
			{Base: model.Base{ID: 3}, MomentID: 9, URL: "moments/7/9/a.jpg"},
			{Base: model.Base{ID: 4}, MomentID: 9, URL: "moments/7/9/b.jpg"},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	resp, err := svc.Delete(9, 7, nil)

	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.ID)
	assert.Equal(t, []string{"moments/7/9/a.jpg", "moments/7/9/b.jpg"}, store.deleteKeys)
}

func TestMomentService_Delete_ReturnsErrorWhenGarageDeleteFails(t *testing.T) {
	store := &fakeMomentObjectStore{deleteErr: errors.New("garage down")}
	repo := &fakeMomentRepo{
		deleteResp: &model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风"},
		deleteMedia: []model.Media{
			{Base: model.Base{ID: 3}, MomentID: 9, URL: "moments/7/9/a.jpg"},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	_, err := svc.Delete(9, 7, nil)

	require.EqualError(t, err, "garage down")
}

func TestMomentService_Delete_SucceedsWithNoMedia(t *testing.T) {
	store := &fakeMomentObjectStore{}
	repo := &fakeMomentRepo{
		deleteResp: &model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风"},
	}
	svc := momentservice.NewMomentService(repo, store, nil, nil, nil, nil)

	resp, err := svc.Delete(9, 7, nil)

	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.ID)
	assert.Empty(t, store.deleteKeys)
}

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func smallPNG(t *testing.T) []byte {
	t.Helper()
	return noisyPNG(t, 8, 8)
}

func noisyPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x*17 + y*31) % 256),
				G: uint8((x*29 + y*11) % 256),
				B: uint8((x*7 + y*19) % 256),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func smallWebP(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)
	return data
}

func TestMomentService_Save_AllowsAdminManagedAuthor(t *testing.T) {
	authorID := uint(99)
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 99, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{UserID: &authorID, Content: "风", Status: 1, CommentStatus: 1}, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.Equal(t, uint(99), repo.saveData.Moment.UserID)
	assert.True(t, repo.saveData.Force)
}

func TestMomentService_Save_RejectsBlankContent(t *testing.T) {
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, &fakeURLResolver{}, nil, nil, nil, nil)

	_, err := svc.Save(dto.MomentSaveReq{Content: "  ", Status: 1, CommentStatus: 1}, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentContentRequired)
}

func TestMomentService_SetTop_MapsLimitError(t *testing.T) {
	repo := &fakeMomentRepo{topErr: momentrepo.ErrTopLimitExceeded}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil, nil, nil, nil)

	_, err := svc.SetTop(9, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentTopLimitExceeded)
}

func TestMomentService_List_ReturnsUnknownError(t *testing.T) {
	repo := &fakeMomentRepo{listErr: errors.New("db down")}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil, nil, nil, nil)

	_, err := svc.List(dto.MomentListReq{}, nil)

	require.EqualError(t, err, "db down")
}

// recordingPublisher 记录发布事件，用于断言点赞是否发布通知。
type recordingPublisher struct {
	events []notificationservice.PublishEvent
}

func (p *recordingPublisher) Publish(_ context.Context, e notificationservice.PublishEvent) (*model.NotificationEvent, error) {
	p.events = append(p.events, e)
	return &model.NotificationEvent{}, nil
}

// 自己点赞自己的碎语不产生通知事件。
func TestMomentService_ToggleLike_SelfLikeDoesNotPublish(t *testing.T) {
	repo := &fakeMomentRepo{
		likeResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 8}, UserID: 5, Content: "碎语"},
		},
	}
	pub := &recordingPublisher{}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil, pub, nil, nil)

	// 碎语作者与点赞者同为 userID=5。
	repo.likeID = 0
	repo.likeUserID = 0

	_, err := svc.ToggleLike(8, 5)

	require.NoError(t, err)
	assert.Empty(t, pub.events)
}

// 点赞别人的碎语正常发布通知事件。
func TestMomentService_ToggleLike_PublishesForOtherAuthor(t *testing.T) {
	repo := &fakeMomentRepo{
		likeResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 8}, UserID: 1, Content: "碎语"},
		},
	}
	pub := &recordingPublisher{}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil, pub, nil, nil)

	_, err := svc.ToggleLike(8, 5)

	require.NoError(t, err)
	require.Len(t, pub.events, 1)
	assert.Equal(t, notificationservice.EventTypeMomentLiked, pub.events[0].Type)
}
