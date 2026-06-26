package music_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/music"
	"github.com/vpt/blog-backend/internal/model"
	musicservice "github.com/vpt/blog-backend/internal/service/music"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubMusicService struct {
	resp   *dto.MusicListResp
	detail *dto.MusicDetailResp
	err    error
}

func (s *stubMusicService) List() (*dto.MusicListResp, error) {
	return s.resp, s.err
}

func (s *stubMusicService) ListPublic() (*dto.MusicListResp, error) {
	return s.List()
}

func (s *stubMusicService) GetPublicDetail(uint) (*dto.MusicDetailResp, error) {
	return s.detail, s.err
}

func (s *stubMusicService) MusicItemsToDTO([]model.Music) ([]dto.MusicItemResp, error) {
	return nil, s.err
}

func (s *stubMusicService) ListArtists(string) (*dto.MusicArtistListResp, error) {
	return &dto.MusicArtistListResp{}, s.err
}

func (s *stubMusicService) GetPublicArtist(uint) (*dto.MusicArtistResp, error) {
	return nil, s.err
}

func (s *stubMusicService) ListAlbums(string) (*dto.MusicAlbumListResp, error) {
	return &dto.MusicAlbumListResp{}, s.err
}

func (s *stubMusicService) GetPublicAlbum(uint) (*dto.MusicAlbumResp, error) {
	return nil, s.err
}

func (s *stubMusicService) ListAdmin(dto.MusicAdminListReq) (*dto.MusicAdminListResp, error) {
	return &dto.MusicAdminListResp{}, s.err
}

func (s *stubMusicService) SaveMusic(context.Context, uint, dto.MusicSaveReq) error {
	return s.err
}

func (s *stubMusicService) DeleteMusic(uint) error {
	return s.err
}

func (s *stubMusicService) SaveArtist(context.Context, uint, dto.MusicArtistSaveReq) (*dto.MusicArtistResp, error) {
	return nil, s.err
}

func (s *stubMusicService) DeleteArtist(uint) error {
	return s.err
}

func (s *stubMusicService) SaveAlbum(context.Context, uint, dto.MusicAlbumSaveReq) (*dto.MusicAlbumResp, error) {
	return nil, s.err
}

func (s *stubMusicService) DeleteAlbum(uint) error {
	return s.err
}

func (s *stubMusicService) UploadAudio(context.Context, musicservice.MusicAudioUploadInput) (*dto.MusicUploadResp, error) {
	return nil, s.err
}

func (s *stubMusicService) UploadAlbumCover(context.Context, musicservice.MusicImageUploadInput) (*dto.MusicUploadResp, error) {
	return nil, s.err
}

func (s *stubMusicService) UploadArtistAvatar(context.Context, musicservice.MusicImageUploadInput) (*dto.MusicUploadResp, error) {
	return nil, s.err
}

var _ musicservice.MusicService = (*stubMusicService)(nil)

func newMusicRouter(svc musicservice.MusicService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := music.NewMusicHandler(svc)
	r.GET("/music", h.List)
	return r
}

func TestMusicHandler_List_Success(t *testing.T) {
	r := newMusicRouter(&stubMusicService{
		resp: &dto.MusicListResp{List: []dto.MusicItemResp{{ID: 1, Name: "Song"}}},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/music", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestMusicHandler_List_ServerError(t *testing.T) {
	r := newMusicRouter(&stubMusicService{err: errors.New("db down")})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/music", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestMusicHandler_GetPublicDetail_Success(t *testing.T) {
	svc := &stubMusicService{detail: &dto.MusicDetailResp{MusicItemResp: dto.MusicItemResp{ID: 1, Name: "Song"}}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := music.NewMusicHandler(svc)
	r.GET("/music/:id", h.GetPublicDetail)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/music/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMusicHandler_SaveArtist_BadJSON(t *testing.T) {
	svc := &stubMusicService{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := music.NewMusicHandler(svc)
	r.POST("/admin/music/artists", h.SaveArtist)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/music/artists", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMusicHandler_SaveMusic_BadJSON(t *testing.T) {
	svc := &stubMusicService{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := music.NewMusicHandler(svc)
	r.POST("/admin/music", h.SaveMusic)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/music", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
