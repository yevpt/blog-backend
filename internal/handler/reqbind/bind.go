package reqbind

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/pkg/response"
)

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
