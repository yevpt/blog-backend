package reqbind

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/pkg/response"
)

const maxIdempotencyKeyChars = 128

const (
	// DefaultJSONMaxBytes 是普通 JSON 接口的统一请求体上限。
	DefaultJSONMaxBytes int64 = 1 << 20
	// ArticleJSONMaxBytes 是文章保存等长正文接口的请求体上限。
	ArticleJSONMaxBytes int64 = 4 << 20
)

// JSON 绑定并校验 JSON 请求体；失败时直接返回可读错误响应。
func JSON(c *gin.Context, req any) bool {
	return JSONWithLimit(c, req, DefaultJSONMaxBytes)
}

// JSONWithLimit 绑定并校验 JSON 请求体，并在解析前限制请求体总大小。
func JSONWithLimit(c *gin.Context, req any, maxBytes int64) bool {
	ensureValidatorLabels()
	if !guardJSONBody(c, maxBytes, true) {
		return false
	}

	if err := c.ShouldBindJSON(req); err != nil {
		if isBodyTooLarge(err) {
			response.Fail(c, response.CodeBadRequest, "请求体过大")
			return false
		}
		response.Fail(c, response.CodeBadRequest, translateBindingError(err))
		return false
	}

	return true
}

// SilentJSON 绑定 JSON 请求体，失败时不写响应，适用于统计上报等静默接口。
func SilentJSON(c *gin.Context, req any) bool {
	ensureValidatorLabels()
	if !guardJSONBody(c, DefaultJSONMaxBytes, false) {
		return false
	}
	return c.ShouldBindJSON(req) == nil
}

func guardJSONBody(c *gin.Context, maxBytes int64, writeError bool) bool {
	if maxBytes <= 0 || c.Request == nil || c.Request.Body == nil {
		return true
	}
	if c.Request.ContentLength > maxBytes {
		if writeError {
			response.Fail(c, response.CodeBadRequest, "请求体过大")
		}
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	return true
}

func isBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

// Query 绑定并校验 Query/Form 参数；失败时直接返回可读错误响应。
func Query(c *gin.Context, req any) bool {
	ensureValidatorLabels()

	if err := c.ShouldBindQuery(req); err != nil {
		response.Fail(c, response.CodeBadRequest, translateBindingError(err))
		return false
	}

	return true
}

// Form 绑定并校验 Form/multipart 请求；失败时直接返回可读错误响应。
func Form(c *gin.Context, req any) bool {
	ensureValidatorLabels()

	if err := c.ShouldBind(req); err != nil {
		response.Fail(c, response.CodeBadRequest, translateBindingError(err))
		return false
	}

	return true
}

// PathUint 解析路径中的正整数参数；失败时直接返回可读错误响应。
func PathUint(c *gin.Context, name string, label string) (uint, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, label+" 必须是大于 0 的整数")
		return 0, false
	}

	return uint(id), true
}

// IdempotencyKey 读取并校验审核写接口要求的幂等键。
func IdempotencyKey(c *gin.Context) (string, bool) {
	return IdempotencyKeyIf(c, true)
}

// IdempotencyKeyIf 在审核写入开启时校验幂等键；关闭时允许省略。
func IdempotencyKeyIf(c *gin.Context, required bool) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		if !required {
			return "", true
		}
		response.Fail(c, response.CodeBadRequest, "Idempotency-Key 请求头缺失或格式不正确")
		return "", false
	}
	if len([]rune(key)) > maxIdempotencyKeyChars || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		response.Fail(c, response.CodeBadRequest, "Idempotency-Key 请求头缺失或格式不正确")
		return "", false
	}
	return key, true
}
