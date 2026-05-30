package response

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Response 是所有 API 接口的统一响应结构
type Response struct {
	Code    int         `json:"code"`            // 0=成功，非0=业务错误
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`  // 成功时携带数据，失败时可省略
}

// 业务错误码约定（以后扩展在此处添加）
const (
	CodeOK              = 0
	CodeBadRequest      = 400
	CodeUnauth          = 401
	CodeForbidden       = 403
	CodeNotFound        = 404
	CodeTooManyRequests = 429
	CodeServerError     = 500
)

// Success 返回成功响应，HTTP 200
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeOK,
		Message: "ok",
		Data:    data,
	})
}

// Fail 返回业务失败响应，HTTP 200 但 code 非 0
// 适用于：参数校验失败、数据不存在、业务规则不满足等
func Fail(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: message,
	})
}

// Unauthorized 返回 401，token 缺失或无效
func Unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    CodeUnauth,
		Message: "未登录或 token 已过期",
	})
}

// Forbidden 返回 403，已登录但权限不足
func Forbidden(c *gin.Context) {
	c.JSON(http.StatusForbidden, Response{
		Code:    CodeForbidden,
		Message: "权限不足",
	})
}

// NotFound 返回 404
func NotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, Response{
		Code:    CodeNotFound,
		Message: "资源不存在",
	})
}

// ServerError 返回 500
func ServerError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, Response{
		Code:    CodeServerError,
		Message: "服务器内部错误",
	})
}

// TooManyRequests 返回 429，并写入 Retry-After header（秒数）
func TooManyRequests(c *gin.Context, message string, retryAfterSeconds int) {
	c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
	c.JSON(http.StatusTooManyRequests, Response{
		Code:    CodeTooManyRequests,
		Message: message,
	})
}
