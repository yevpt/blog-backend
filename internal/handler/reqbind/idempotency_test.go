package reqbind_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
