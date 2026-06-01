package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/internal/repository/mock"
	"github.com/vpt/blog-backend/internal/service"
)

func TestCategoryService_ListTabs_MapsFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := service.NewCategoryService(repo)

	url := "tech"
	repo.EXPECT().
		ListWithArticleCount().
		Return([]repository.CategoryWithCount{
			{
				Category:     model.Category{Base: model.Base{ID: 1}, Name: "编程", URL: &url, Seq: 0},
				ArticleCount: 5,
			},
			{
				Category:     model.Category{Base: model.Base{ID: 2}, Name: "工具", Seq: 1},
				ArticleCount: 3,
			},
		}, nil)

	resp, err := svc.ListTabs()
	require.NoError(t, err)
	require.Len(t, resp.List, 2)

	assert.Equal(t, uint(1), resp.List[0].ID)
	assert.Equal(t, "编程", resp.List[0].Name)
	assert.Equal(t, &url, resp.List[0].URL)
	assert.Equal(t, uint(0), resp.List[0].Seq)
	assert.Equal(t, int64(5), resp.List[0].ArticleCount)

	assert.Equal(t, uint(2), resp.List[1].ID)
	assert.Nil(t, resp.List[1].URL)
	assert.Equal(t, int64(3), resp.List[1].ArticleCount)
}

func TestCategoryService_ListTabs_EmptyResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := service.NewCategoryService(repo)

	repo.EXPECT().ListWithArticleCount().Return([]repository.CategoryWithCount{}, nil)

	resp, err := svc.ListTabs()
	require.NoError(t, err)
	assert.Empty(t, resp.List)
}

func TestCategoryService_ListTabs_PropagatesRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := service.NewCategoryService(repo)

	dbErr := errors.New("db error")
	repo.EXPECT().ListWithArticleCount().Return(nil, dbErr)

	_, err := svc.ListTabs()
	require.ErrorIs(t, err, dbErr)
}

func TestCategoryService_Create_TrimsAndRequiresFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := service.NewCategoryService(repo)

	seq := uint(2)
	icon := "icon-key"
	desc := "描述"
	cover := "cover-key"
	repo.EXPECT().
		Create(gomock.Any()).
		DoAndReturn(func(category model.Category) (*repository.CategoryWithCount, error) {
			assert.Equal(t, "编程", category.Name)
			assert.Equal(t, seq, category.Seq)
			assert.Equal(t, &icon, category.Icon)
			assert.Equal(t, &desc, category.Description)
			assert.Equal(t, &cover, category.CoverImgUrl)
			return &repository.CategoryWithCount{Category: model.Category{Base: model.Base{ID: 3}, Name: category.Name, Seq: category.Seq}}, nil
		})

	resp, err := svc.Create(dto.CategoryCreateReq{
		Name:        "  编程  ",
		Seq:         &seq,
		Icon:        icon,
		Description: desc,
		CoverImgUrl: cover,
	})
	require.NoError(t, err)
	assert.Equal(t, uint(3), resp.ID)
	assert.Equal(t, "编程", resp.Name)
}

func TestCategoryService_Create_BlankNameReturnsBadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := service.NewCategoryService(repo)

	seq := uint(0)
	_, err := svc.Create(dto.CategoryCreateReq{
		Name:        " ",
		Seq:         &seq,
		Icon:        "icon",
		Description: "desc",
		CoverImgUrl: "cover",
	})
	require.ErrorIs(t, err, service.ErrCategoryNameRequired)
}

func TestCategoryService_AddArticles_NormalizesIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := service.NewCategoryService(repo)

	repo.EXPECT().
		AddArticles(uint(5), []uint{8, 9}).
		Return(int64(2), nil)

	resp, err := svc.AddArticles(5, dto.CategoryArticlesReq{ArticleIDs: []uint{8, 0, 9, 8}})
	require.NoError(t, err)
	assert.Equal(t, uint(5), resp.CategoryID)
	assert.Equal(t, []uint{8, 9}, resp.ArticleIDs)
	assert.Equal(t, int64(2), resp.AffectedCount)
}

func TestCategoryService_AddArticles_RequiresIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := service.NewCategoryService(repo)

	_, err := svc.AddArticles(5, dto.CategoryArticlesReq{ArticleIDs: []uint{0, 0}})
	require.ErrorIs(t, err, service.ErrCategoryArticleRequired)
}
