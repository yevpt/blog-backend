package moderation_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
)

func TestCreateImportRejectsExcessFileParts(t *testing.T) {
	handler := newRuleAdminHandler(t, nil)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"format":             "csv",
		"source_name":        "测试来源",
		"default_category":   "fraud",
		"default_effect":     "review",
		"default_risk_level": "medium",
	} {
		require.NoError(t, writer.WriteField(key, value))
	}
	for _, field := range []string{"file", "extra"} {
		part, err := writer.CreateFormFile(field, field+".csv")
		require.NoError(t, err)
		_, err = part.Write([]byte("pattern\n测试\n"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/moderation/rule-imports", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	jwtpkg.SetClaims(ctx, &jwtpkg.Claims{UserId: 1})

	handler.CreateImport(ctx)

	assert.Contains(t, recorder.Body.String(), "上传文件过多")
}
