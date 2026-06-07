package moment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	momentservice "github.com/vpt/blog-backend/internal/service/moment"
	"github.com/vpt/blog-backend/pkg/roles"
)

type fakeMomentRepo struct {
	listFilter   momentrepo.ListFilter
	listViewerID *uint
	listResp     *momentrepo.PageResult
	listErr      error

	saveData momentrepo.SaveData
	saveResp *momentrepo.MomentAggregate
	saveErr  error

	deleteID       uint
	deleteOperator uint
	deleteForce    bool
	deleteResp     *model.Moment
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
}

func (f *fakeMomentRepo) List(filter momentrepo.ListFilter, viewerID *uint) (*momentrepo.PageResult, error) {
	f.listFilter = filter
	f.listViewerID = viewerID
	return f.listResp, f.listErr
}

func (f *fakeMomentRepo) FindPublicDetail(uint, *uint) (*momentrepo.MomentAggregate, error) {
	return nil, momentrepo.ErrMomentNotFound
}

func (f *fakeMomentRepo) Save(data momentrepo.SaveData) (*momentrepo.MomentAggregate, error) {
	f.saveData = data
	return f.saveResp, f.saveErr
}

func (f *fakeMomentRepo) Delete(id uint, operatorID uint, force bool) (*model.Moment, error) {
	f.deleteID = id
	f.deleteOperator = operatorID
	f.deleteForce = force
	return f.deleteResp, f.deleteErr
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

type fakeURLResolver struct {
	objects []string
}

func (r *fakeURLResolver) ObjectURL(_ context.Context, objectName string) (string, error) {
	r.objects = append(r.objects, objectName)
	return "https://cdn.example.com/" + objectName, nil
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
				Images: []model.Media{{Base: model.Base{ID: 3}, OwnerID: 9, URL: "moments/cat.jpg", Name: "cat.jpg"}},
			}},
		},
	}
	resolver := &fakeURLResolver{}
	svc := momentservice.NewMomentService(repo, resolver, nil)

	resp, err := svc.List(dto.MomentListReq{Page: 0, PageSize: 99}, &viewerID)

	require.NoError(t, err)
	assert.Equal(t, 1, repo.listFilter.Page)
	assert.Equal(t, 50, repo.listFilter.PageSize)
	assert.Equal(t, &viewerID, repo.listViewerID)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "https://cdn.example.com/moments/cat.jpg", resp.List[0].Images[0].AccessURL)
	assert.Equal(t, []string{"moments/cat.jpg"}, resolver.objects)
}

func TestMomentService_Save_TrimsContentAndUsesCurrentUserForNormalRole(t *testing.T) {
	now := time.Now()
	requestUserID := uint(99)
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9, CreatedAt: now, UpdatedAt: now}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil)

	resp, err := svc.Save(dto.MomentSaveReq{
		UserID:        &requestUserID,
		Content:       "  风  ",
		Status:        1,
		CommentStatus: 1,
		Images:        []dto.MomentMediaReq{{Name: "cat.jpg", URL: "moments/cat.jpg", Size: 10}},
	}, 7, nil)

	require.NoError(t, err)
	assert.Equal(t, uint(7), repo.saveData.Moment.UserID)
	assert.Equal(t, "风", repo.saveData.Moment.Content)
	assert.False(t, repo.saveData.Force)
	assert.Equal(t, uint(7), repo.saveData.OperatorID)
	assert.Equal(t, "moments/cat.jpg", repo.saveData.Images[0].URL)
	assert.Equal(t, uint(9), resp.ID)
}

func TestMomentService_Save_AllowsAdminManagedAuthor(t *testing.T) {
	authorID := uint(99)
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 99, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil)

	_, err := svc.Save(dto.MomentSaveReq{UserID: &authorID, Content: "风", Status: 1, CommentStatus: 1}, 7, []string{roles.AdminRole})

	require.NoError(t, err)
	assert.Equal(t, uint(99), repo.saveData.Moment.UserID)
	assert.True(t, repo.saveData.Force)
}

func TestMomentService_Save_RejectsBlankContent(t *testing.T) {
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, &fakeURLResolver{}, nil)

	_, err := svc.Save(dto.MomentSaveReq{Content: "  ", Status: 1, CommentStatus: 1}, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentContentRequired)
}

func TestMomentService_SetTop_MapsLimitError(t *testing.T) {
	repo := &fakeMomentRepo{topErr: momentrepo.ErrTopLimitExceeded}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil)

	_, err := svc.SetTop(9, 7, nil)

	require.ErrorIs(t, err, momentservice.ErrMomentTopLimitExceeded)
}

func TestMomentService_List_ReturnsUnknownError(t *testing.T) {
	repo := &fakeMomentRepo{listErr: errors.New("db down")}
	svc := momentservice.NewMomentService(repo, &fakeURLResolver{}, nil)

	_, err := svc.List(dto.MomentListReq{}, nil)

	require.EqualError(t, err, "db down")
}
