package multipartlimit_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/handler/multipartlimit"
	"github.com/vpt/blog-backend/pkg/response"
)

func TestGuard_RejectsContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/upload", nil)
	c.Request.ContentLength = multipartlimit.SingleFileMaxBody(1024) + 1

	ok := multipartlimit.Guard(c, multipartlimit.SingleFileMaxBody(1024))
	assert.False(t, ok)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, multipartlimit.ErrBodyTooLarge.Error(), resp.Message)
}

func TestGuard_RejectsOversizedBodyOnParse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const maxFile = 128
	maxBody := multipartlimit.SingleFileMaxBody(maxFile)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "big.bin")
	require.NoError(t, err)
	_, err = part.Write(bytes.Repeat([]byte("x"), int(maxBody)+1))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/upload", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request.ContentLength = -1
	require.True(t, multipartlimit.Guard(c, maxBody))

	_, err = c.FormFile("file")
	require.Error(t, err)
	assert.True(t, multipartlimit.RespondParseError(c, err))

	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}
