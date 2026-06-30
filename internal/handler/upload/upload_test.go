package upload_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	uploadhandler "github.com/vpt/blog-backend/internal/handler/upload"
	"github.com/vpt/blog-backend/internal/handler/multipartlimit"
	uploadservice "github.com/vpt/blog-backend/internal/service/upload"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubUploadService struct {
	resp  *dto.TempUploadResp
	err   error
	input uploadservice.TempImageInput
}

func (s *stubUploadService) UploadTempImage(ctx context.Context, input uploadservice.TempImageInput) (*dto.TempUploadResp, error) {
	s.input = input
	return s.resp, s.err
}

func newUploadRouter(svc uploadservice.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := uploadhandler.NewHandler(svc)
	r.POST("/uploads/temp", h.TempImage)
	r.POST("/authed/uploads/temp", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		h.TempImage(c)
	})
	return r
}

func TestHandler_TempImage_RejectsMissingAuth(t *testing.T) {
	r := newUploadRouter(&stubUploadService{})
	body, contentType := tempUploadBody(t, "images", "cat.png", []byte("png"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/uploads/temp", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeUnauth, resp.Code)
}

func TestHandler_TempImage_RejectsInvalidDir(t *testing.T) {
	stub := &stubUploadService{err: uploadservice.ErrUploadDirInvalid}
	r := newUploadRouter(stub)
	body, contentType := tempUploadBody(t, "../images", "cat.png", []byte("png"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/authed/uploads/temp", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, uploadservice.ErrUploadDirInvalid.Error(), resp.Message)
}

func TestHandler_TempImage_Success(t *testing.T) {
	stub := &stubUploadService{
		resp: &dto.TempUploadResp{
			Key: "temp/articles/7/images/a.png",
			URL: "https://cdn.example.com/blog/temp/articles/7/images/a.png",
		},
	}
	r := newUploadRouter(stub)
	fileData := []byte("fake-image")
	body, contentType := tempUploadBody(t, "images", "cat.png", fileData)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/authed/uploads/temp", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.input.UserID)
	assert.Equal(t, "images", stub.input.Dir)
	assert.Equal(t, "cat.png", stub.input.Name)
	assert.Equal(t, fileData, stub.input.Data)

	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeOK, resp.Code)
}

func TestHandler_TempImage_ReturnsServerErrorOnUnknownError(t *testing.T) {
	r := newUploadRouter(&stubUploadService{err: errors.New("boom")})
	body, contentType := tempUploadBody(t, "images", "cat.png", []byte("png"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/authed/uploads/temp", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_TempImage_RejectsOversizedBody(t *testing.T) {
	r := newUploadRouter(&stubUploadService{})
	oversized := bytes.Repeat([]byte("x"), int(multipartlimit.SingleFileMaxBody(uploadservice.MaxTempImageBytes))+1)
	body, contentType := tempUploadBody(t, "images", "big.png", oversized)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/authed/uploads/temp", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, multipartlimit.ErrBodyTooLarge.Error(), resp.Message)
}

func tempUploadBody(t *testing.T, dir string, name string, data []byte) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("dir", dir))
	fileWriter, err := writer.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = fileWriter.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}
