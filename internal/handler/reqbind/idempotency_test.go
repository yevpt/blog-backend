package reqbind_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

func TestIdempotencyKeyValidatesRequiredHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		key  string
		ok   bool
	}{
		{name: "missing"},
		{name: "control", key: "bad\nkey"},
		{name: "too long", key: strings.Repeat("a", 129)},
		{name: "valid", key: " publish-1 ", ok: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/", nil)
			ctx.Request.Header.Set("Idempotency-Key", test.key)

			got, ok := reqbind.IdempotencyKey(ctx)

			assert.Equal(t, test.ok, ok)
			if test.ok {
				assert.Equal(t, "publish-1", got)
			} else {
				assert.Equal(t, 200, recorder.Code)
				assert.Contains(t, recorder.Body.String(), "Idempotency-Key")
			}
		})
	}
}

func TestIdempotencyKeyIfAllowsMissingWhenOptional(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/", nil)

	got, ok := reqbind.IdempotencyKeyIf(ctx, false)

	assert.True(t, ok)
	assert.Empty(t, got)
	assert.Equal(t, 0, recorder.Body.Len())
}

func TestJSONRejectsBodyAboveDefaultLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"`+strings.Repeat("a", int(reqbind.DefaultJSONMaxBytes))+`"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	var req struct {
		Value string `json:"value"`
	}
	ok := reqbind.JSON(ctx, &req)

	assert.False(t, ok)
	assert.Equal(t, 200, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "请求体过大")
}

func TestJSONWithLimitAllowsLargerExplicitLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"value":"` + strings.Repeat("a", int(reqbind.DefaultJSONMaxBytes)) + `"}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	var req struct {
		Value string `json:"value"`
	}
	ok := reqbind.JSONWithLimit(ctx, &req, reqbind.ArticleJSONMaxBytes)

	require.True(t, ok)
	assert.Len(t, req.Value, int(reqbind.DefaultJSONMaxBytes))
	assert.Equal(t, 0, recorder.Body.Len())
}
