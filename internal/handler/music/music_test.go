package music_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/music"
	musicservice "github.com/vpt/blog-backend/internal/service/music"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubMusicService struct {
	resp *dto.MusicListResp
	err  error
}

func (s *stubMusicService) List() (*dto.MusicListResp, error) {
	return s.resp, s.err
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
