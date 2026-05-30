package response

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Response 所有 API 接口的统一响应包装
type Response struct {
	Code    int         `json:"code"`           // 0 表示成功，非 0 为业务错误码
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // 失败时省略，不输出 null
}

// 业务错误码，与 HTTP 状态码对齐，便于客户端统一处理
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

// Fail 返回业务失败响应，HTTP 状态码固定 200，由 code 字段表达错误类型
func Fail(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: message,
	})
}

// Unauthorized 返回 401，用于 token 缺失、格式错误或已过期
func Unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    CodeUnauth,
		Message: "未登录或 token 已过期",
	})
}

// Forbidden 返回 403，身份已验证但角色权限不足
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

// TooManyRequests 返回 429，同时写入 Retry-After header，告知客户端最早重试时间
func TooManyRequests(c *gin.Context, message string, retryAfterSeconds int) {
	c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
	c.JSON(http.StatusTooManyRequests, Response{
		Code:    CodeTooManyRequests,
		Message: message,
	})
}
