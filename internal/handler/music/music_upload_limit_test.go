package music_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/music"
	musicservice "github.com/vpt/blog-backend/internal/service/music"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type trackingMusicUploadService struct {
	stubMusicService
	uploadAudioCalled bool
}

func (s *trackingMusicUploadService) UploadAudio(context.Context, musicservice.MusicAudioUploadInput) (*dto.MusicUploadResp, error) {
	s.uploadAudioCalled = true
	return &dto.MusicUploadResp{}, nil
}

func TestMusicHandler_UploadAudio_RejectsExcessFileParts(t *testing.T) {
	svc := &trackingMusicUploadService{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := music.NewMusicHandler(svc)
	r.POST("/admin/music/uploads/audio", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 1})
		c.Next()
	}, h.UploadAudio)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, field := range []string{"file", "extra"} {
		part, err := writer.CreateFormFile(field, field+".mp3")
		require.NoError(t, err)
		_, err = part.Write([]byte("audio"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/music/uploads/audio", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)

	assert.False(t, svc.uploadAudioCalled)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, "上传文件过多", resp.Message)
}
