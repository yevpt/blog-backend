package moderation_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	moderationhandler "github.com/vpt/blog-backend/internal/handler/moderation"
	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
	rulemod "github.com/vpt/blog-backend/internal/service/moderationrule"
	rulemock "github.com/vpt/blog-backend/internal/service/moderationrule/mock"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

func TestRuleTemplateSetsSafeAttachmentHeaders(t *testing.T) {
	handler := newRuleAdminHandler(t, nil)
	recorder := serveRuleAdmin(http.MethodGet, "/admin/moderation/rule-imports/template?format=csv", "", handler.DownloadTemplate, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/csv; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "moderation-rules-template.csv")
	assert.Contains(t, recorder.Body.String(), "pattern")
}

func TestRuleTemplateTXTFormat(t *testing.T) {
	handler := newRuleAdminHandler(t, nil)
	recorder := serveRuleAdmin(http.MethodGet, "/admin/moderation/rule-imports/template?format=txt", "", handler.DownloadTemplate, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/plain")
}

func TestRuleTemplateRejectsInvalidFormat(t *testing.T) {
	handler := newRuleAdminHandler(t, nil)
	recorder := serveRuleAdmin(http.MethodGet, "/admin/moderation/rule-imports/template?format=zip", "", handler.DownloadTemplate, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":400`)
}

func TestCreateRuleUsesJWTActorAndExpectedVersion(t *testing.T) {
	svc, handler := newRuleAdminHandlerWithMock(t)
	svc.EXPECT().CreateRule(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, cmd rulemod.CreateRuleCommand) (rulemod.Job, error) {
		assert.Equal(t, uint64(1), cmd.ActorID)
		assert.Equal(t, uint64(7), cmd.ExpectedRulesetID)
		assert.Equal(t, "keyword", cmd.Rule.RuleType)
		return rulemod.Job{RulesetID: 8, BaseRulesetID: 7, Status: "building"}, nil
	})

	body := `{"expected_ruleset_version":7,"rule_type":"keyword","pattern":"风险词","category":"other","effect":"review","risk_level":"medium","priority":100,"source_id":1}`
	recorder := serveRuleAdmin(http.MethodPost, "/admin/moderation/rules", body, handler.CreateRule, true)

	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"ruleset_id":8`)
}

func TestCreateRuleRequiresJWT(t *testing.T) {
	_, handler := newRuleAdminHandlerWithMock(t)
	body := `{"expected_ruleset_version":7,"rule_type":"keyword","pattern":"风险词","category":"other","effect":"review","risk_level":"medium","priority":100,"source_id":1}`
	recorder := serveRuleAdmin(http.MethodPost, "/admin/moderation/rules", body, handler.CreateRule, false)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestCreateRuleConflictReturns409(t *testing.T) {
	svc, handler := newRuleAdminHandlerWithMock(t)
	svc.EXPECT().CreateRule(gomock.Any(), gomock.Any()).Return(rulemod.Job{}, rulemod.ErrRulesetConflict)

	body := `{"expected_ruleset_version":7,"rule_type":"keyword","pattern":"风险词","category":"other","effect":"review","risk_level":"medium","priority":100,"source_id":1}`
	recorder := serveRuleAdmin(http.MethodPost, "/admin/moderation/rules", body, handler.CreateRule, true)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), response.CodeModerationRulesetConflict)
}

func TestCreateRuleRejectsInvalidBody(t *testing.T) {
	_, handler := newRuleAdminHandlerWithMock(t)
	body := `{"expected_ruleset_version":0}`
	recorder := serveRuleAdmin(http.MethodPost, "/admin/moderation/rules", body, handler.CreateRule, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":400`)
}

func TestListRulesReturnsCursorPage(t *testing.T) {
	svc, handler := newRuleAdminHandlerWithMock(t)
	svc.EXPECT().ListRules(gomock.Any(), gomock.Any()).Return(repoMod.RulePage{
		Rules:      []repoMod.RuleListRecord{{ID: 1, RuleType: "keyword", Pattern: "测试"}},
		NextCursor: 2, HasMore: true,
	}, nil)

	recorder := serveRuleAdmin(http.MethodGet, "/admin/moderation/rules?limit=1&search_mode=prefix&pattern=测", "", handler.ListRules, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"next_cursor":2`)
	assert.Contains(t, recorder.Body.String(), `"has_more":true`)
}

func TestGetRuleStatusReturnsCurrentAndCandidate(t *testing.T) {
	svc, handler := newRuleAdminHandlerWithMock(t)
	failureCode := "build_failed"
	svc.EXPECT().Status(gomock.Any()).Return(rulemod.Status{
		CurrentRulesetID: 7, RuleCount: 1000,
		Candidate: &rulemod.CandidateStatus{RulesetID: 8, Status: "ready", FailureCode: &failureCode},
	}, nil)

	recorder := serveRuleAdmin(http.MethodGet, "/admin/moderation/rules/status", "", handler.GetRuleStatus, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"current_ruleset_id":7`)
	assert.Contains(t, recorder.Body.String(), `"candidate"`)
}

func TestGetRuleMetadataReturnsCategories(t *testing.T) {
	svc, handler := newRuleAdminHandlerWithMock(t)
	svc.EXPECT().Metadata(gomock.Any()).Return(rulemod.Metadata{
		Categories: []rulemod.CategoryEntry{{Key: "fraud", Name: "欺诈"}},
		RiskLevels: []string{"low", "medium", "high"},
	}, nil)

	recorder := serveRuleAdmin(http.MethodGet, "/admin/moderation/rules/metadata", "", handler.GetRuleMetadata, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"fraud"`)
	assert.Contains(t, recorder.Body.String(), `"欺诈"`)
}

func TestBatchStatusRejectsEmptyIDs(t *testing.T) {
	_, handler := newRuleAdminHandlerWithMock(t)
	body := `{"expected_ruleset_version":7,"rule_ids":[],"active":false}`
	recorder := serveRuleAdmin(http.MethodPost, "/admin/moderation/rules/batch-status", body, handler.BatchStatus, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":400`)
}

func TestCancelImportReturns404ForMissing(t *testing.T) {
	svc, handler := newRuleAdminHandlerWithMock(t)
	svc.EXPECT().CancelImport(gomock.Any(), uint64(99), uint64(1)).Return(rulemod.ErrCandidateNotFound)

	recorder := serveRuleAdminWithParams(http.MethodDelete, "/admin/moderation/rule-imports/99", "", handler.CancelImport, true, gin.Params{{Key: "id", Value: "99"}})

	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestCreateImportAcceptsMultipartFile(t *testing.T) {
	svc, handler := newRuleAdminHandlerWithMock(t)
	svc.EXPECT().CreateImport(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input rulemod.CreateImportInput) (repoMod.ImportRecord, error) {
		body, err := io.ReadAll(input.Body)
		assert.NoError(t, err)
		assert.Equal(t, "rules.csv", input.FileName)
		assert.Equal(t, "csv", input.Format)
		assert.Equal(t, "pattern\n测试\n", string(body))
		assert.Equal(t, uint64(1), input.OperatorID)
		return repoMod.ImportRecord{ID: 10, FileName: input.FileName}, nil
	})

	recorder := serveRuleImportMultipart(handler, "rules.csv", "pattern\n测试\n", "csv")

	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"id":10`)
}

func TestCreateImportRejectsMissingAndEmptyFile(t *testing.T) {
	_, handler := newRuleAdminHandlerWithMock(t)

	missing := serveRuleImportMultipart(handler, "", "", "csv")
	empty := serveRuleImportMultipart(handler, "rules.csv", "", "csv")

	assert.Contains(t, missing.Body.String(), `"code":400`)
	assert.Contains(t, empty.Body.String(), `"code":400`)
}

func TestCreateImportRejectsOversizedMultipartBody(t *testing.T) {
	_, handler := newRuleAdminHandlerWithMock(t)
	handler.SetRuleImportMaxFileBytes(4)

	recorder := serveRuleImportMultipart(handler, "rules.csv", "pattern\n测试\n", "csv")

	assert.Contains(t, recorder.Body.String(), `"code":400`)
	assert.Contains(t, recorder.Body.String(), "上传内容过大")
}

func TestRuleTextTestReturnsHits(t *testing.T) {
	svc, handler := newRuleAdminHandlerWithMock(t)
	svc.EXPECT().TestText(gomock.Any(), gomock.Any()).Return(rulemod.TestResult{
		Risk:      "high",
		RulesetID: 7,
		RuleIDs:   []uint64{5},
		Hits: []rulemod.TestHit{{
			RuleID: 5, RuleType: "keyword", Pattern: "风险",
			Category: "fraud", RiskLevel: "high", Effect: "review", Excerpt: "风险",
		}},
	}, nil)

	body := `{"text":"这是一段风险文本"}`
	recorder := serveRuleAdmin(http.MethodPost, "/admin/moderation/rules/test", body, handler.TestRuleText, true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"risk":"high"`)
	assert.Contains(t, recorder.Body.String(), `"rule_id":5`)
}

func newRuleAdminHandler(t *testing.T, svc rulemod.Service) *moderationhandler.AdminHandler {
	t.Helper()
	handler := moderationhandler.NewAdminHandler(nil, nil)
	if svc != nil {
		handler.SetRuleService(svc)
	} else {
		handler.SetRuleService(&noOpRuleService{})
	}
	return handler
}

func newRuleAdminHandlerWithMock(t *testing.T) (*rulemock.MockService, *moderationhandler.AdminHandler) {
	t.Helper()
	ctrl := gomock.NewController(t)
	svc := rulemock.NewMockService(ctrl)
	handler := moderationhandler.NewAdminHandler(nil, nil)
	handler.SetRuleService(svc)
	return svc, handler
}

func serveRuleAdmin(method, path, body string, action gin.HandlerFunc, withClaims bool) *httptest.ResponseRecorder {
	return serveRuleAdminWithParams(method, path, body, action, withClaims, nil)
}

func serveRuleAdminWithParams(method, path, body string, action gin.HandlerFunc, withClaims bool, params gin.Params) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	if len(params) > 0 {
		ctx.Params = params
	}
	if withClaims {
		jwtpkg.SetClaims(ctx, &jwtpkg.Claims{UserId: 1})
	}
	action(ctx)
	return recorder
}

func serveRuleImportMultipart(handler *moderationhandler.AdminHandler, filename, content, format string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("format", format)
	_ = writer.WriteField("source_name", "测试来源")
	_ = writer.WriteField("default_category", "fraud")
	_ = writer.WriteField("default_effect", "review")
	_ = writer.WriteField("default_risk_level", "medium")
	_ = writer.WriteField("default_priority", "100")
	if filename != "" {
		part, _ := writer.CreateFormFile("file", filename)
		_, _ = part.Write([]byte(content))
	}
	_ = writer.Close()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/moderation/rule-imports", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	jwtpkg.SetClaims(ctx, &jwtpkg.Claims{UserId: 1})
	handler.CreateImport(ctx)
	return recorder
}

type noOpRuleService struct{}

func (s *noOpRuleService) ListRules(context.Context, rulemod.ListQuery) (repoMod.RulePage, error) {
	return repoMod.RulePage{}, nil
}
func (s *noOpRuleService) Metadata(context.Context) (rulemod.Metadata, error) {
	return rulemod.Metadata{}, nil
}
func (s *noOpRuleService) Status(context.Context) (rulemod.Status, error) {
	return rulemod.Status{}, nil
}
func (s *noOpRuleService) CreateRule(context.Context, rulemod.CreateRuleCommand) (rulemod.Job, error) {
	return rulemod.Job{}, nil
}
func (s *noOpRuleService) ReplaceRule(context.Context, rulemod.ReplaceRuleCommand) (rulemod.Job, error) {
	return rulemod.Job{}, nil
}
func (s *noOpRuleService) BatchStatus(context.Context, rulemod.BatchStatusCommand) (rulemod.Job, error) {
	return rulemod.Job{}, nil
}
func (s *noOpRuleService) TestText(context.Context, rulemod.TestTextCommand) (rulemod.TestResult, error) {
	return rulemod.TestResult{}, nil
}
func (s *noOpRuleService) PublishCandidate(context.Context, uint64, uint64, uint64) error {
	return nil
}
func (s *noOpRuleService) CancelCandidate(context.Context, uint64, uint64) error {
	return nil
}
func (s *noOpRuleService) CreateImport(context.Context, rulemod.CreateImportInput) (repoMod.ImportRecord, error) {
	return repoMod.ImportRecord{}, nil
}
func (s *noOpRuleService) ListImports(context.Context, uint64, int) (repoMod.ImportPage, error) {
	return repoMod.ImportPage{}, nil
}
func (s *noOpRuleService) GetImport(context.Context, uint64) (repoMod.ImportRecord, error) {
	return repoMod.ImportRecord{}, nil
}
func (s *noOpRuleService) CancelImport(context.Context, uint64, uint64) error {
	return nil
}
