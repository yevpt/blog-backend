package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/internal/repository/mock"
	"github.com/vpt/blog-backend/internal/service"
	"gorm.io/gorm"
)

func TestArticleService_ListPublic_NormalizesPagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	repo.EXPECT().
		ListPublic(repository.ArticleListFilter{Page: 1, PageSize: 50}).
		Return(&repository.ArticlePageResult{Total: 0, Page: 1, PageSize: 50}, nil)

	resp, err := svc.ListPublic(dto.ArticleListReq{Page: -1, PageSize: 99})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 50, resp.PageSize)
}

func TestArticleService_ListPublic_ResolvesCoverImgURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	resolver := &stubObjectURLResolver{
		urls: map[string]string{
			"post/bg-images/202106/245eb60be3b9dadf181b6e98ae7482f6.jpg": "https://cdn.example.com/blog/post/bg-images/202106/245eb60be3b9dadf181b6e98ae7482f6.jpg?a=sign&b=1700000000",
		},
	}
	svc := service.NewArticleService(repo, resolver)

	cover := "post/bg-images/202106/245eb60be3b9dadf181b6e98ae7482f6.jpg"
	repo.EXPECT().
		ListPublic(repository.ArticleListFilter{Page: 1, PageSize: 10}).
		Return(&repository.ArticlePageResult{
			Total:    1,
			Page:     1,
			PageSize: 10,
			Articles: []repository.ArticleAggregate{{
				Article: model.Article{
					Base:        model.Base{ID: 1},
					Title:       "Cover",
					CoverImgUrl: &cover,
					UserID:      1,
					Status:      1,
				},
			}},
		}, nil)

	resp, err := svc.ListPublic(dto.ArticleListReq{})
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	require.NotNil(t, resp.List[0].CoverImgUrl)
	assert.Equal(t, resolver.urls[cover], *resp.List[0].CoverImgUrl)
	assert.Equal(t, []string{cover}, resolver.objectNames)
}

func TestArticleService_ListPublic_IncludesCategoryInListItem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	categoryURL := "tech"
	repo.EXPECT().
		ListPublic(repository.ArticleListFilter{Page: 1, PageSize: 10}).
		Return(&repository.ArticlePageResult{
			Total:    1,
			Page:     1,
			PageSize: 10,
			Articles: []repository.ArticleAggregate{{
				Article: model.Article{
					Base:   model.Base{ID: 1},
					Title:  "Hello",
					UserID: 1,
					Status: 1,
				},
				Categories: []model.Category{
					{Base: model.Base{ID: 3}, Name: "Tech", URL: &categoryURL},
				},
			}},
		}, nil)

	resp, err := svc.ListPublic(dto.ArticleListReq{})
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	require.NotNil(t, resp.List[0].Category)
	assert.Equal(t, uint(3), resp.List[0].Category.ID)
	assert.Equal(t, "Tech", resp.List[0].Category.Name)
	assert.Equal(t, &categoryURL, resp.List[0].Category.URL)
}

func TestArticleService_ListPublic_NilCategoryWhenNoneAssigned(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	repo.EXPECT().
		ListPublic(repository.ArticleListFilter{Page: 1, PageSize: 10}).
		Return(&repository.ArticlePageResult{
			Total:    1,
			Page:     1,
			PageSize: 10,
			Articles: []repository.ArticleAggregate{{
				Article: model.Article{
					Base:   model.Base{ID: 1},
					Title:  "No Category",
					UserID: 1,
					Status: 1,
				},
				Categories: nil,
			}},
		}, nil)

	resp, err := svc.ListPublic(dto.ArticleListReq{})
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Nil(t, resp.List[0].Category)
}

func TestArticleService_SaveRejectsEncryptedArticleWithoutPassword(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	_, err := svc.Save(dto.ArticleSaveReq{
		Title:         "Secret",
		Content:       "body",
		Status:        2,
		CommentStatus: 1,
		CategoryIDs:   []uint{1},
	}, 1)
	require.ErrorIs(t, err, service.ErrArticlePasswordRequired)
}

func TestArticleService_SaveDeduplicatesRelationIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	now := time.Now()
	repo.EXPECT().
		Save(gomock.Any()).
		DoAndReturn(func(data repository.ArticleSaveData) (*repository.ArticleAggregate, error) {
			assert.Equal(t, []uint{1, 2}, data.CategoryIDs)
			assert.Equal(t, []uint{3, 4}, data.TagIDs)
			assert.Equal(t, []uint{5, 6}, data.MusicIDs)
			return &repository.ArticleAggregate{
				Article: model.Article{
					Base:          model.Base{ID: 9, CreatedAt: now, UpdatedAt: now},
					Title:         data.Article.Title,
					Content:       data.Article.Content,
					UserID:        data.Article.UserID,
					Status:        data.Article.Status,
					CommentStatus: data.Article.CommentStatus,
				},
			}, nil
		})

	resp, err := svc.Save(dto.ArticleSaveReq{
		Title:         "A",
		Content:       "body",
		Status:        1,
		CommentStatus: 1,
		CategoryIDs:   []uint{1, 1, 2},
		TagIDs:        []uint{3, 3, 4},
		MusicIDs:      []uint{5, 5, 6},
	}, 7)
	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.ID)
}

func TestArticleService_GetPublicDetail_HidesEncryptedContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	repo.EXPECT().
		FindPublicDetail(uint(2), (*uint)(nil)).
		Return(&repository.ArticleAggregate{
			Article: model.Article{
				Base:    model.Base{ID: 2},
				Title:   "Secret",
				Content: "hidden body",
				UserID:  1,
				Status:  2,
			},
		}, nil)

	resp, err := svc.GetPublicDetail(2, nil)
	require.NoError(t, err)
	assert.True(t, resp.Passworded)
	assert.Empty(t, resp.Content)
}

func TestArticleService_GetAdminDetail_IncludesEncryptedContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	repo.EXPECT().
		FindAdminDetail(uint(2), (*uint)(nil)).
		Return(&repository.ArticleAggregate{
			Article: model.Article{
				Base:    model.Base{ID: 2},
				Title:   "Secret",
				Content: "admin body",
				UserID:  1,
				Status:  2,
			},
		}, nil)

	resp, err := svc.GetAdminDetail(2, nil)
	require.NoError(t, err)
	assert.True(t, resp.Passworded)
	assert.Equal(t, "admin body", resp.Content)
}

func TestArticleService_GetPublicDetail_MapsAggregateFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	now := time.Now()
	categoryURL := "tech"
	tagURL := "go"
	musicURL := "https://example.com/song.mp3"
	recommendSeq := uint(8)
	viewerID := uint(10)
	repo.EXPECT().
		FindPublicDetail(uint(3), &viewerID).
		Return(&repository.ArticleAggregate{
			Article: model.Article{
				Base:          model.Base{ID: 3, CreatedAt: now, UpdatedAt: now},
				Title:         "A",
				Content:       "body",
				UserID:        1,
				Status:        1,
				CommentStatus: 1,
				ReadCount:     11,
			},
			Categories: []model.Category{{Base: model.Base{ID: 4}, Name: "Tech", URL: &categoryURL}},
			Tags:       []model.Tag{{Base: model.Base{ID: 5}, Name: "Go", URL: &tagURL}},
			Music:      []model.Music{{Base: model.Base{ID: 6}, Name: "Song", URL: &musicURL, Duration: 240}},
			Recommend:  &model.ArticleRecommend{ArticleID: 3, Seq: recommendSeq},
			LikeCount:  7,
			IsLiked:    true,
		}, nil)

	resp, err := svc.GetPublicDetail(3, &viewerID)
	require.NoError(t, err)
	assert.Equal(t, "body", resp.Content)
	assert.Equal(t, int64(7), resp.LikeCount)
	assert.True(t, resp.IsLiked)
	assert.True(t, resp.IsRecommended)
	require.NotNil(t, resp.RecommendSeq)
	assert.Equal(t, recommendSeq, *resp.RecommendSeq)
	assert.Equal(t, []uint{4}, resp.CategoryIDs)
	assert.Equal(t, []uint{5}, resp.TagIDs)
	assert.Equal(t, []uint{6}, resp.MusicIDs)
	assert.Equal(t, "Tech", resp.Categories[0].Name)
	assert.Equal(t, "Go", resp.Tags[0].Name)
	assert.Equal(t, uint16(240), resp.Music[0].Duration)
}

func TestArticleService_GetPublicDetail_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	repo.EXPECT().
		FindPublicDetail(uint(404), (*uint)(nil)).
		Return(nil, nil)

	_, err := svc.GetPublicDetail(404, nil)
	require.ErrorIs(t, err, service.ErrArticleNotFound)
}

func TestArticleService_IsLiked_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockArticleRepository(ctrl)
	svc := service.NewArticleService(repo, nil)

	repo.EXPECT().
		IsLiked(uint(8), uint(1)).
		Return(false, int64(0), gorm.ErrRecordNotFound)

	_, err := svc.IsLiked(8, 1)
	require.ErrorIs(t, err, service.ErrArticleNotFound)
	assert.True(t, errors.Is(err, service.ErrArticleNotFound))
}

type stubObjectURLResolver struct {
	urls        map[string]string
	objectNames []string
}

func (r *stubObjectURLResolver) ObjectURL(_ context.Context, objectName string) (string, error) {
	r.objectNames = append(r.objectNames, objectName)
	return r.urls[objectName], nil
}
