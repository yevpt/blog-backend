package response

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// FormatRetryAfter 把秒数转成用户友好的时间描述
func FormatRetryAfter(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	minutes := seconds / 60
	if seconds%60 == 0 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	return fmt.Sprintf("约 %d 分钟", minutes)
}

// Response 所有 API 接口的统一响应包装
type Response struct {
	Code      int    `json:"code"` // 0 表示成功，非 0 为业务错误码
	ErrorCode string `json:"error_code,omitempty" example:"CONTENT_RISK_REJECTED"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"` // 失败时省略，不输出 null
}

const (
	CodeContentRiskRejected         = "CONTENT_RISK_REJECTED"
	CodeImageReviewUnavailable      = "CONTENT_IMAGE_REVIEW_UNAVAILABLE"
	CodeContentAlreadyDeleted       = "CONTENT_ALREADY_DELETED"
	CodeContentPendingNoInteraction = "CONTENT_PENDING_NO_INTERACTION"
	CodeModerationReviewConflict    = "MODERATION_REVIEW_CONFLICT"
	CodeModerationRulesetConflict   = "MODERATION_RULESET_CONFLICT"
	CodeModerationRuleLimit         = "MODERATION_RULE_LIMIT"
	CodeModerationIndexMemoryLimit  = "MODERATION_INDEX_MEMORY_LIMIT"
	CodeModerationImportInvalid     = "MODERATION_IMPORT_INVALID"
	CodeAuthEmailTaken              = "AUTH_EMAIL_TAKEN"
)

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
func Success(c *gin.Context, data any) {
	message := "ok"
	if provider, ok := data.(interface{ ResponseMessage() string }); ok && provider.ResponseMessage() != "" {
		message = provider.ResponseMessage()
	}
	SuccessWithMessage(c, data, message)
}

// SuccessWithMessage 返回带业务提示的成功响应。
func SuccessWithMessage(c *gin.Context, data any, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeOK,
		Message: message,
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
	AuthFailed(c, "未登录或 token 已过期")
}

// AuthFailed 返回 401，用于登录凭证错误等认证失败场景。
func AuthFailed(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    CodeUnauth,
		Message: message,
	})
}

// Forbidden 返回 403，身份已验证但角色权限不足
func Forbidden(c *gin.Context) {
	ForbiddenWithMessage(c, "权限不足")
}

// ForbiddenWithMessage 返回 403，并允许业务场景指定可展示的禁止访问原因。
func ForbiddenWithMessage(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Response{
		Code:    CodeForbidden,
		Message: message,
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

// Conflict 返回审核状态冲突。
func Conflict(c *gin.Context, errorCode string, message string) {
	c.JSON(http.StatusConflict, Response{Code: http.StatusConflict, ErrorCode: errorCode, Message: message})
}

// UnprocessableEntity 返回内容可解析但因风险不能接受。
func UnprocessableEntity(c *gin.Context, errorCode string, message string) {
	c.JSON(http.StatusUnprocessableEntity, Response{Code: http.StatusUnprocessableEntity, ErrorCode: errorCode, Message: message})
}
