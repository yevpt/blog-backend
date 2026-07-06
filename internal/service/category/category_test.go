package category_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	categoryrepo "github.com/vpt/blog-backend/internal/repository/category"
	"github.com/vpt/blog-backend/internal/repository/category/mock"
	"github.com/vpt/blog-backend/internal/service/category"
)

// ---- 现有测试（保持不变，这里只是确认旧行为继续有效）----

func TestCategoryService_ListTabs_MapsFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := category.NewCategoryService(repo, nil, nil)

	url := "tech"
	repo.EXPECT().
		ListWithArticleCount().
		Return([]categoryrepo.CategoryWithCount{
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
	svc := category.NewCategoryService(repo, nil, nil)

	repo.EXPECT().ListWithArticleCount().Return([]categoryrepo.CategoryWithCount{}, nil)

	resp, err := svc.ListTabs()
	require.NoError(t, err)
	assert.Empty(t, resp.List)
}

func TestCategoryService_ListTabs_PropagatesRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := category.NewCategoryService(repo, nil, nil)

	dbErr := errors.New("db error")
	repo.EXPECT().ListWithArticleCount().Return(nil, dbErr)

	_, err := svc.ListTabs()
	require.ErrorIs(t, err, dbErr)
}

// ---- 新增：可选字段测试 ----

func TestCategoryService_Create_AllOptionalFieldsEmpty_Succeeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := category.NewCategoryService(repo, nil, nil)

	seq := uint(0)
	repo.EXPECT().
		CreateWithPrepare(gomock.Any()).
		DoAndReturn(func(data categoryrepo.CategoryCreateData) (*categoryrepo.CategoryWithCount, error) {
			assert.Equal(t, "编程", data.Category.Name)
			assert.Nil(t, data.Category.Icon)
			assert.Nil(t, data.Category.Description)
			assert.Nil(t, data.Category.CoverImgUrl)
			return &categoryrepo.CategoryWithCount{Category: model.Category{Base: model.Base{ID: 1}, Name: "编程", Seq: 0}}, nil
		})

	resp, err := svc.Create(context.Background(), uint(7), dto.CategoryCreateReq{
		Name: "编程",
		Seq:  &seq,
	})
	require.NoError(t, err)
	assert.Equal(t, uint(1), resp.ID)
}

func TestCategoryService_Create_BlankNameReturnsBadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := category.NewCategoryService(repo, nil, nil)

	seq := uint(0)
	_, err := svc.Create(context.Background(), uint(7), dto.CategoryCreateReq{
		Name: " ",
		Seq:  &seq,
	})
	require.ErrorIs(t, err, category.ErrCategoryNameRequired)
}

func TestCategoryService_AddArticles_NormalizesIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockCategoryRepository(ctrl)
	svc := category.NewCategoryService(repo, nil, nil)

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
	svc := category.NewCategoryService(repo, nil, nil)

	_, err := svc.AddArticles(5, dto.CategoryArticlesReq{ArticleIDs: []uint{0, 0}})
	require.ErrorIs(t, err, category.ErrCategoryArticleRequired)
}
