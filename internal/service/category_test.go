package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
