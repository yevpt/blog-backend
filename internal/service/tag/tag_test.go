package tag_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	tagrepo "github.com/vpt/blog-backend/internal/repository/tag"
	"github.com/vpt/blog-backend/internal/repository/tag/mock"
	articleservice "github.com/vpt/blog-backend/internal/service/article"
	"github.com/vpt/blog-backend/internal/service/tag"
)

type stubArticleServiceForTag struct {
	req dto.ArticleListReq
	res *dto.ArticlePageResp
	err error
}

func (s *stubArticleServiceForTag) ListIDs() (*dto.ArticleIDsResp, error) {
	return nil, nil
}

func (s *stubArticleServiceForTag) ListPublic(req dto.ArticleListReq, viewerID *uint) (*dto.ArticlePageResp, error) {
	s.req = req
	return s.res, s.err
}

func (s *stubArticleServiceForTag) GetPublicDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	return nil, nil
}

func (s *stubArticleServiceForTag) GetAdminDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	return nil, nil
}

func (s *stubArticleServiceForTag) Save(req dto.ArticleSaveReq, authorID uint) (*dto.ArticleDetailResp, error) {
	return nil, nil
}

func (s *stubArticleServiceForTag) Delete(id uint) (*dto.ArticleDetailResp, error) {
	return nil, nil
}

func (s *stubArticleServiceForTag) View(id uint, visitorID string) (*dto.ArticleViewResp, error) {
	return nil, nil
}

func (s *stubArticleServiceForTag) IsLiked(id uint, userID uint) (*dto.ArticleLikeResp, error) {
	return nil, nil
}

func (s *stubArticleServiceForTag) ToggleLike(id uint, userID uint) (*dto.ArticleLikeResp, error) {
	return nil, nil
}

var _ articleservice.ArticleService = (*stubArticleServiceForTag)(nil)

func TestTagService_List_MapsFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockTagRepository(ctrl)
	svc := tag.NewTagService(repo, nil)

	url := "go"
	repo.EXPECT().
		ListWithArticleCount().
		Return([]tagrepo.TagWithCount{
			{
				Tag:          model.Tag{Base: model.Base{ID: 1}, Name: "Go", URL: &url, Seq: 0},
				ArticleCount: 5,
			},
		}, nil)

	resp, err := svc.List()
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, uint(1), resp.List[0].ID)
	assert.Equal(t, "Go", resp.List[0].Name)
	assert.Equal(t, &url, resp.List[0].URL)
	assert.Equal(t, int64(5), resp.List[0].ArticleCount)
}

func TestTagService_Create_TrimsAndRequiresFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockTagRepository(ctrl)
	svc := tag.NewTagService(repo, nil)

	seq := uint(2)
	icon := "icon-key"
	repo.EXPECT().
		Create(gomock.Any()).
		DoAndReturn(func(tag model.Tag) (*tagrepo.TagWithCount, error) {
			assert.Equal(t, "Go", tag.Name)
			assert.Equal(t, seq, tag.Seq)
			assert.Equal(t, &icon, tag.Icon)
			return &tagrepo.TagWithCount{Tag: model.Tag{Base: model.Base{ID: 3}, Name: tag.Name, Seq: tag.Seq}}, nil
		})

	resp, err := svc.Create(dto.TagCreateReq{Name: "  Go  ", Seq: &seq, Icon: &icon})
	require.NoError(t, err)
	assert.Equal(t, uint(3), resp.ID)
	assert.Equal(t, "Go", resp.Name)
}

func TestTagService_Create_BlankNameReturnsBadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockTagRepository(ctrl)
	svc := tag.NewTagService(repo, nil)

	seq := uint(0)
	_, err := svc.Create(dto.TagCreateReq{Name: " ", Seq: &seq})
	require.ErrorIs(t, err, tag.ErrTagNameRequired)
}

func TestTagService_AddArticles_NormalizesIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockTagRepository(ctrl)
	svc := tag.NewTagService(repo, nil)

	repo.EXPECT().
		AddArticles(uint(5), []uint{8, 9}).
		Return(int64(2), nil)

	resp, err := svc.AddArticles(5, dto.TagArticlesReq{ArticleIDs: []uint{8, 0, 9, 8}})
	require.NoError(t, err)
	assert.Equal(t, uint(5), resp.TagID)
	assert.Equal(t, []uint{8, 9}, resp.ArticleIDs)
	assert.Equal(t, int64(2), resp.AffectedCount)
}

func TestTagService_AddArticles_RequiresIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockTagRepository(ctrl)
	svc := tag.NewTagService(repo, nil)

	_, err := svc.AddArticles(5, dto.TagArticlesReq{ArticleIDs: []uint{0, 0}})
	require.ErrorIs(t, err, tag.ErrTagArticleRequired)
}

func TestTagService_ListArticles_RequiresExistingTagAndDelegatesToArticleService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockTagRepository(ctrl)
	articleSvc := &stubArticleServiceForTag{res: &dto.ArticlePageResp{Page: 2, PageSize: 5}}
	svc := tag.NewTagService(repo, articleSvc)

	repo.EXPECT().
		FindWithArticleCount(uint(5)).
		Return(&tagrepo.TagWithCount{Tag: model.Tag{Base: model.Base{ID: 5}, Name: "Go"}}, nil)

	resp, err := svc.ListArticles(5, dto.ArticleListReq{Page: 2, PageSize: 5})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, articleSvc.req.TagID)
	assert.Equal(t, uint(5), *articleSvc.req.TagID)
	assert.Equal(t, 2, articleSvc.req.Page)
	assert.Equal(t, 5, articleSvc.req.PageSize)
}

func TestTagService_ListArticles_MissingTagReturnsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockTagRepository(ctrl)
	articleSvc := &stubArticleServiceForTag{}
	svc := tag.NewTagService(repo, articleSvc)

	repo.EXPECT().
		FindWithArticleCount(uint(5)).
		Return(nil, nil)

	_, err := svc.ListArticles(5, dto.ArticleListReq{})
	require.ErrorIs(t, err, tag.ErrTagNotFound)
}

func TestTagService_List_PropagatesRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock.NewMockTagRepository(ctrl)
	svc := tag.NewTagService(repo, nil)

	dbErr := errors.New("db error")
	repo.EXPECT().ListWithArticleCount().Return(nil, dbErr)

	_, err := svc.List()
	require.ErrorIs(t, err, dbErr)
}
