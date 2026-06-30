package imageupload

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/handler/multipartlimit"
)

// ReadSingleImageFile 从 multipart 表单读取单张图片，并限制读取大小。
func ReadSingleImageFile(c *gin.Context, field string, maxBytes int, rejectGIF bool) (name string, data []byte, err error) {
	if !multipartlimit.Guard(c, multipartlimit.SingleFileMaxBody(maxBytes)) {
		return "", nil, multipartlimit.ErrBodyTooLarge
	}

	header, err := c.FormFile(field)
	if err != nil {
		if multipartlimit.RespondParseError(c, err) {
			return "", nil, multipartlimit.ErrBodyTooLarge
		}
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil, nil
		}
		return "", nil, err
	}
	if multipartlimit.RejectExcessFileParts(c, 1) {
		return "", nil, multipartlimit.ErrTooManyFiles
	}

	file, err := header.Open()
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	data, err = io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > maxBytes {
		return "", nil, fmt.Errorf("上传图片不能超过 %dKB", maxBytes/1024)
	}
	if rejectGIF && isGIFUpload(header.Header.Get("Content-Type"), header.Filename) {
		return "", nil, errors.New("不支持 GIF 头像")
	}
	return header.Filename, data, nil
}

func isGIFUpload(contentType string, fileName string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "image/gif" || strings.EqualFold(filepath.Ext(fileName), ".gif")
}
