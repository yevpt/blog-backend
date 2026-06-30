package friendlink_test

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
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/friendlink"
	friendlinkservice "github.com/vpt/blog-backend/internal/service/friendlink"
	"github.com/vpt/blog-backend/pkg/response"
)

type stubFriendLinkService struct {
	listPublicReq dto.FriendLinkListReq
	getErr        error
	createReq     dto.FriendLinkCreateReq
	updateID      uint
	updateReq     dto.FriendLinkUpdateReq
}

func (s *stubFriendLinkService) ListPublic(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error) {
	s.listPublicReq = req
	return &dto.FriendLinkPageResp{Page: req.Page, PageSize: req.PageSize, List: []dto.FriendLinkItemResp{}}, nil
}

func (s *stubFriendLinkService) GetPublic(id uint) (*dto.FriendLinkItemResp, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &dto.FriendLinkItemResp{ID: id, Name: "友站"}, nil
}

func (s *stubFriendLinkService) ListAdmin(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error) {
	return &dto.FriendLinkPageResp{Page: req.Page, PageSize: req.PageSize, List: []dto.FriendLinkItemResp{}}, nil
}

func (s *stubFriendLinkService) Create(req dto.FriendLinkCreateReq) (*dto.FriendLinkItemResp, error) {
	s.createReq = req
	return &dto.FriendLinkItemResp{ID: 3, Name: req.Name}, nil
}

func (s *stubFriendLinkService) Update(id uint, req dto.FriendLinkUpdateReq) (*dto.FriendLinkItemResp, error) {
	s.updateID = id
	s.updateReq = req
	return &dto.FriendLinkItemResp{ID: id}, nil
}

func (s *stubFriendLinkService) Delete(id uint) (*dto.FriendLinkItemResp, error) {
	return &dto.FriendLinkItemResp{ID: id}, nil
}

func newFriendLinkRouter(svc friendlinkservice.FriendLinkService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := friendlink.NewFriendLinkHandler(svc)
	r.GET("/friend-links", h.ListPublic)
	r.GET("/friend-links/:id", h.GetPublic)
	r.POST("/admin/friend-links", h.Create)
	r.PUT("/admin/friend-links/:id", h.Update)
	return r
}

func TestFriendLinkHandler_ListPublic_BindsQuery(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/friend-links?page=2&page_size=5", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 2, stub.listPublicReq.Page)
	assert.Equal(t, 5, stub.listPublicReq.PageSize)
}

func TestFriendLinkHandler_Create_InvalidJSONReturnsBadRequest(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)

	w := httptest.NewRecorder()
	body, contentType := friendLinkMultipartBody(t, map[string]string{
		"name": "友站",
		"site": "https://friend.example.com",
		"seq":  "1",
	}, "", nil)
	req := httptest.NewRequest("POST", "/admin/friend-links", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, "缺少友链 Logo", resp.Message)
}

func TestFriendLinkHandler_Create_BindsMultipartLogoAndFields(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)
	body, contentType := friendLinkMultipartBody(t, map[string]string{
		"name":        "友站",
		"description": "描述",
		"site":        "https://friend.example.com",
		"seq":         "2",
		"status":      "1",
	}, "logo.png", []byte("fake-image"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/friend-links", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "友站", stub.createReq.Name)
	assert.Equal(t, "https://friend.example.com", stub.createReq.Site)
	require.NotNil(t, stub.createReq.Seq)
	assert.Equal(t, uint(2), *stub.createReq.Seq)
	require.NotNil(t, stub.createReq.Logo)
	assert.Equal(t, "logo.png", stub.createReq.Logo.Name)
	assert.Equal(t, []byte("fake-image"), stub.createReq.Logo.Data)
}

func TestFriendLinkHandler_Create_RejectsTooLargeLogo(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)
	body, contentType := friendLinkMultipartBody(t, map[string]string{
		"name": "友站",
		"site": "https://friend.example.com",
		"seq":  "1",
	}, "logo.png", bytes.Repeat([]byte("x"), friendlinkservice.MaxFriendLinkLogoBytes+1))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/friend-links", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, stub.createReq.Name)

	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, friendlinkservice.ErrFriendLinkLogoTooLarge.Error(), resp.Message)
}

func TestFriendLinkHandler_Create_RejectsExcessFileParts(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("name", "友站"))
	require.NoError(t, writer.WriteField("site", "https://friend.example.com"))
	require.NoError(t, writer.WriteField("seq", "1"))
	for _, field := range []string{"logo", "extra"} {
		part, err := writer.CreateFormFile(field, field+".png")
		require.NoError(t, err)
		_, err = part.Write([]byte("fake-image"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/friend-links", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)

	assert.Empty(t, stub.createReq.Name)
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, response.CodeBadRequest, resp.Code)
	assert.Equal(t, "上传文件过多", resp.Message)
}

func TestFriendLinkHandler_GetPublic_NotFoundReturns404(t *testing.T) {
	stub := &stubFriendLinkService{getErr: friendlinkservice.ErrFriendLinkNotFound}
	r := newFriendLinkRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/friend-links/9", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFriendLinkHandler_Update_BindsPathAndBody(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)
	body, contentType := friendLinkMultipartBody(t, map[string]string{
		"name": "友站",
	}, "logo.png", []byte("fake-image"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/admin/friend-links/7", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.updateID)
	require.NotNil(t, stub.updateReq.Name)
	assert.Equal(t, "友站", *stub.updateReq.Name)
	require.NotNil(t, stub.updateReq.Logo)
	assert.Equal(t, "logo.png", stub.updateReq.Logo.Name)
	assert.Equal(t, []byte("fake-image"), stub.updateReq.Logo.Data)
}

func TestFriendLinkHandler_Update_AllowsMissingLogo(t *testing.T) {
	stub := &stubFriendLinkService{}
	r := newFriendLinkRouter(stub)
	body, contentType := friendLinkMultipartBody(t, map[string]string{
		"name": "友站",
	}, "", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/admin/friend-links/7", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.updateID)
	require.NotNil(t, stub.updateReq.Name)
	assert.Equal(t, "友站", *stub.updateReq.Name)
	assert.Nil(t, stub.updateReq.Logo)
}

func friendLinkMultipartBody(t *testing.T, fields map[string]string, fileName string, fileData []byte) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	if fileName != "" {
		fileWriter, err := writer.CreateFormFile("logo", fileName)
		require.NoError(t, err)
		_, err = fileWriter.Write(fileData)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}
