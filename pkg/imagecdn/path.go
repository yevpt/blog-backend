package imagecdn

import (
	"fmt"
	"strings"
)

// ObjectKeyFromCDNPath 从 CDN 请求 path（/{bucket}/{objectKey}）解析对象 key。
func ObjectKeyFromCDNPath(bucket, requestPath string) (string, error) {
	bucket = strings.Trim(bucket, "/")
	if bucket == "" {
		return "", fmt.Errorf("bucket 不能为空")
	}

	path := strings.TrimSpace(requestPath)
	if path == "" {
		return "", fmt.Errorf("请求 path 不能为空")
	}

	prefix := "/" + bucket + "/"
	if !strings.HasPrefix(path, prefix) {
		return "", fmt.Errorf("path 与 bucket 不匹配")
	}

	objectKey := strings.TrimLeft(path[len(prefix):], "/")
	if objectKey == "" {
		return "", fmt.Errorf("对象 key 不能为空")
	}

	return objectKey, nil
}
