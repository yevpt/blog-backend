package moment_test

import (
	"bytes"
	"context"
	"crypto/md5"
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
	removed  []string

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
}

func (s *fakeMomentObjectStore) ObjectExists(_ context.Context, objectName string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.exists[objectName], nil
}

func (s *fakeMomentObjectStore) PutObject(_ context.Context, objectName string, data []byte, _ string) error {
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
	s.uploadedData[objectName] = append([]byte(nil), data...)
	return nil
}

func (s *fakeMomentObjectStore) DeleteObject(_ context.Context, objectName string) error {
	s.deleteKeys = append(s.deleteKeys, objectName)
	return s.deleteErr
}

func (s *fakeMomentObjectStore) MoveObject(context.Context, string, string) error {
	return nil
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
	store := &fakeMomentObjectStore{
		exists: map[string]bool{"moments/old.jpg": true},
	}
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9, CreatedAt: now, UpdatedAt: now}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil)

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
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil)

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
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil)

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
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil)
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
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil)

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

func TestMomentService_Save_CompressesLargeImageToFiveHundredKB(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{}}
	repo := &fakeMomentRepo{
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil)

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

func TestMomentService_Save_DeletesRemovedOldImagesAfterSuccessfulSave(t *testing.T) {
	store := &fakeMomentObjectStore{exists: map[string]bool{"moments/7/9/keep.jpg": true}}
	repo := &fakeMomentRepo{
		removed: []string{"https://cdn.example.com/blog/moments/7/9/remove.jpg?sign=1"},
		saveResp: &momentrepo.MomentAggregate{
			Moment: model.Moment{Base: model.Base{ID: 9}, UserID: 7, Content: "风", Status: 1, CommentStatus: 1},
		},
	}
	svc := momentservice.NewMomentService(repo, store, nil)

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
	svc := momentservice.NewMomentService(repo, store, nil)

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
	svc := momentservice.NewMomentService(&fakeMomentRepo{}, store, nil)

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
