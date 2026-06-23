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
	"github.com/vpt/blog-backend/pkg/response"
)

// stubAdminService 记录管理端 service 入参。
type stubAdminService struct {
	taskReq   dto.AdminNotificationListReq
	batchReq  dto.AdminNotificationListReq
	retryID   uint
	retryResp *dto.AdminBatchRetryResp
}

func (s *stubAdminService) ListEmailTasks(req dto.AdminNotificationListReq) (*dto.AdminEmailTaskPageResp, error) {
	s.taskReq = req
	return &dto.AdminEmailTaskPageResp{Page: req.Page, PageSize: req.PageSize}, nil
}
func (s *stubAdminService) ListEmailBatches(req dto.AdminNotificationListReq) (*dto.AdminEmailBatchPageResp, error) {
	s.batchReq = req
	return &dto.AdminEmailBatchPageResp{Page: req.Page, PageSize: req.PageSize}, nil
}
func (s *stubAdminService) ListQuotas() (*dto.AdminQuotaListResp, error) {
	return &dto.AdminQuotaListResp{}, nil
}
func (s *stubAdminService) UpdateQuota(uint, dto.AdminUpdateQuotaReq) error         { return nil }
func (s *stubAdminService) UpdateRoleQuota(uint, dto.AdminUpdateRoleQuotaReq) error { return nil }
func (s *stubAdminService) RetryBatch(id uint) (*dto.AdminBatchRetryResp, error) {
	s.retryID = id
	return s.retryResp, nil
}

func newAdminRouter(svc *stubAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := notificationhandler.NewNotificationAdminHandler(svc)
	r.GET("/admin/notifications/email-tasks", h.ListEmailTasks)
	r.GET("/admin/notifications/email-batches", h.ListEmailBatches)
	r.PUT("/admin/notifications/email-quotas/:id", h.UpdateQuota)
	r.POST("/admin/notifications/email-batches/:id/retry", h.RetryBatch)
	return r
}

func decodeAdmin(t *testing.T, w *httptest.ResponseRecorder) response.Response {
	t.Helper()
	var resp response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// 邮件任务列表绑定状态与分页。
func TestAdminHandler_ListEmailTasks_BindsStatusAndPaging(t *testing.T) {
	stub := &stubAdminService{}
	r := newAdminRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/notifications/email-tasks?page=2&page_size=20&status=pending", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 2, stub.taskReq.Page)
	assert.Equal(t, 20, stub.taskReq.PageSize)
	assert.Equal(t, "pending", stub.taskReq.Status)
}

// 邮件批次列表绑定状态与分页。
func TestAdminHandler_ListEmailBatches_BindsStatusAndPaging(t *testing.T) {
	stub := &stubAdminService{}
	r := newAdminRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/notifications/email-batches?page=3&page_size=15&status=failed", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 3, stub.batchReq.Page)
	assert.Equal(t, 15, stub.batchReq.PageSize)
	assert.Equal(t, "failed", stub.batchReq.Status)
}

// 额度调整拒绝负数或越界值。
func TestAdminHandler_UpdateQuota_RejectsNegativeValues(t *testing.T) {
	stub := &stubAdminService{}
	r := newAdminRouter(stub)

	body, _ := json.Marshal(dto.AdminUpdateQuotaReq{DailyLimit: -1})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/admin/notifications/email-quotas/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decodeAdmin(t, w).Code)
}

// 重试批次通过 service 改状态。
func TestAdminHandler_RetryBatch_CallsService(t *testing.T) {
	stub := &stubAdminService{retryResp: &dto.AdminBatchRetryResp{ID: 9, Status: "pending"}}
	r := newAdminRouter(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/notifications/email-batches/9/retry", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint(9), stub.retryID)
	assert.Equal(t, response.CodeOK, decodeAdmin(t, w).Code)
}
