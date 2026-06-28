package reqbind

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/pkg/response"
)

const maxIdempotencyKeyChars = 128

// JSON 绑定并校验 JSON 请求体；失败时直接返回可读错误响应。
func JSON(c *gin.Context, req any) bool {
	ensureValidatorLabels()

	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, response.CodeBadRequest, translateBindingError(err))
		return false
	}

	return true
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
