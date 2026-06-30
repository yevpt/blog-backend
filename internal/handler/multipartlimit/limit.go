// Package multipartlimit 在解析 multipart 前限制请求体总大小，避免先完整解析再单文件 LimitReader。
package multipartlimit

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/pkg/response"
)

const (
	// 单文件 multipart 表单字段与 boundary 开销估算。
	singleFileFormOverhead = 64 * 1024
	// 多文件每个 part 的 boundary 与头开销估算。
	multiFilePartOverhead = 4 * 1024
	// 多文件表单非文件字段开销估算。
	multiFileBaseOverhead = 128 * 1024
)

// ErrBodyTooLarge 表示 multipart 请求体超过硬限制。
var ErrBodyTooLarge = errors.New("上传内容过大")

// ErrTooManyFiles 表示 multipart 中文件 part 数量超过硬限制。
var ErrTooManyFiles = errors.New("上传文件过多")

// Guard 在解析 multipart 前限制请求体总大小；若 Content-Length 已超限则直接拒绝。
func Guard(c *gin.Context, maxTotalBytes int64) bool {
	if maxTotalBytes <= 0 {
		return true
	}
	if cl := c.Request.ContentLength; cl > 0 && cl > maxTotalBytes {
		response.Fail(c, response.CodeBadRequest, ErrBodyTooLarge.Error())
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxTotalBytes)
	return true
}

// SingleFileMaxBody 计算单文件上传允许的最大请求体。
func SingleFileMaxBody(maxFileBytes int) int64 {
	return int64(maxFileBytes) + singleFileFormOverhead
}

// MultiFileMaxBody 计算多文件上传允许的最大请求体。
func MultiFileMaxBody(maxFiles int, maxFileBytes int) int64 {
	if maxFiles <= 0 {
		maxFiles = 1
	}
	return multiFileBaseOverhead + int64(maxFiles)*(int64(maxFileBytes)+multiFilePartOverhead)
}

// RespondParseError 处理 multipart 解析错误；若为请求体过大则写入响应并返回 true。
func RespondParseError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		response.Fail(c, response.CodeBadRequest, ErrBodyTooLarge.Error())
		return true
	}
	return false
}

// RejectExcessFileParts 在 multipart 已解析后拒绝过多文件 part。
func RejectExcessFileParts(c *gin.Context, maxFiles int) bool {
	if maxFiles <= 0 {
		maxFiles = 1
	}
	form := c.Request.MultipartForm
	if form == nil {
		return false
	}
	total := 0
	for _, files := range form.File {
		total += len(files)
	}
	if total > maxFiles {
		response.Fail(c, response.CodeBadRequest, "上传文件过多")
		return true
	}
	return false
}