package notification_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	notificationhandler "github.com/vpt/blog-backend/internal/handler/notification"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/pkg/response"
)

// stubInboxService 记录 service 入参，驱动 handler 行为断言。
type stubInboxService struct {
	listUserID uint
	listReq    dto.NotificationListReq
	listResp   *dto.NotificationPageResp

	unreadUserID uint
	unreadResp   *dto.NotificationUnreadCountResp

	markReadUserID uint
	markReadID     uint
	markReadErr    error

	markAllUserID uint
	markAllIDs    []uint
	markAllCalled bool

	deleteUserID uint
	deleteID     uint
}

func (s *stubInboxService) List(userID uint, req dto.NotificationListReq) (*dto.NotificationPageResp, error) {
	s.listUserID = userID
	s.listReq = req
	return s.listResp, nil
}

func (s *stubInboxService) UnreadCount(userID uint) (*dto.NotificationUnreadCountResp, error) {
	s.unreadUserID = userID
	return s.unreadResp, nil
}

func (s *stubInboxService) MarkRead(userID uint, id uint) error {
	s.markReadUserID = userID
	s.markReadID = id
	return s.markReadErr
}

func (s *stubInboxService) MarkAllRead(userID uint, ids []uint) (*dto.NotificationReadResp, error) {
	s.markAllUserID = userID
	s.markAllIDs = ids
	s.markAllCalled = true
	return &dto.NotificationReadResp{Updated: int64(len(ids))}, nil
}

func (s *stubInboxService) Delete(userID uint, id uint) error {
	s.deleteUserID = userID
	s.deleteID = id
	return nil
}

// newRouter 构造带模拟登录态的路由；authed 为 false 时不写入用户详情，模拟未登录。
func newRouter(svc *stubInboxService, authed bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := notificationhandler.NewNotificationHandler(svc)

	withAuth := func(next gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			if authed {
				middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "vpt", Status: 1})
			}
			next(c)
		}
	}

	r.GET("/notifications", withAuth(h.List))
	r.GET("/notifications/unread-count", withAuth(h.UnreadCount))
	r.PATCH("/notifications/read", withAuth(h.MarkAllRead))
	r.PATCH("/notifications/:id/read", withAuth(h.MarkRead))
	r.DELETE("/notifications/:id", withAuth(h.Delete))
	return r
}

func decodeResp(t *testing.T, w *httptest.ResponseRecorder) response.Response {
	t.Helper()
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// 列表应绑定分页与 unread_only，并透传当前用户 ID。
func TestNotificationHandler_List_BindsPaginationAndUser(t *testing.T) {
	stub := &stubInboxService{listResp: &dto.NotificationPageResp{Page: 1, PageSize: 10}}
	r := newRouter(stub, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/notifications?page=2&page_size=5&unread_only=true", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.listUserID)
	assert.Equal(t, 2, stub.listReq.Page)
	assert.Equal(t, 5, stub.listReq.PageSize)
	assert.True(t, stub.listReq.UnreadOnly)
}

// 未读数接口必须登录，未登录返回 401。
func TestNotificationHandler_UnreadCount_RequiresAuth(t *testing.T) {
	stub := &stubInboxService{unreadResp: &dto.NotificationUnreadCountResp{Count: 3}}
	r := newRouter(stub, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/notifications/unread-count", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, uint(0), stub.unreadUserID)
}

// 非法路径 ID 返回 400 参数错误，不调用 service。
func TestNotificationHandler_MarkRead_RejectsInvalidID(t *testing.T) {
	stub := &stubInboxService{}
	r := newRouter(stub, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/notifications/abc/read", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decodeResp(t, w).Code)
	assert.Equal(t, uint(0), stub.markReadID)
}

// 批量已读：ids 超过 100 上限返回 400，不调用 service。
func TestNotificationHandler_MarkAllRead_RejectsIDListAboveLimit(t *testing.T) {
	stub := &stubInboxService{}
	r := newRouter(stub, true)

	ids := make([]uint, 101)
	for i := range ids {
		ids[i] = uint(i + 1)
	}
	body, _ := json.Marshal(dto.NotificationReadAllReq{IDs: ids})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/notifications/read", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decodeResp(t, w).Code)
	assert.False(t, stub.markAllCalled)
}

// 批量已读：既未给 ids 也未声明 all 时返回 400，避免误清空。
func TestNotificationHandler_MarkAllRead_RejectsEmptyScope(t *testing.T) {
	stub := &stubInboxService{}
	r := newRouter(stub, true)

	body, _ := json.Marshal(dto.NotificationReadAllReq{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/notifications/read", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, response.CodeBadRequest, decodeResp(t, w).Code)
	assert.False(t, stub.markAllCalled)
}

// 删除应只用当前登录用户 ID 调用 service。
func TestNotificationHandler_Delete_UsesCurrentUserID(t *testing.T) {
	stub := &stubInboxService{}
	r := newRouter(stub, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/notifications/15", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(7), stub.deleteUserID)
	assert.Equal(t, uint(15), stub.deleteID)
}
