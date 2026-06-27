package avatar

import (
	"strings"

	"github.com/vpt/blog-backend/pkg/storage"
)

// ResolveManagedAvatarKey 从对象 key 或本站对象 URL 解析出托管头像 key。
func ResolveManagedAvatarKey(resolver storage.ObjectKeyResolver, value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if IsManagedAvatarKey(value) {
		return strings.TrimLeft(value, "/"), true
	}
	if resolver == nil || !storage.IsAbsoluteURL(value) {
		return "", false
	}
	key, err := resolver.ObjectKey(value)
	if err != nil || !IsManagedAvatarKey(key) {
		return "", false
	}
	return strings.TrimLeft(key, "/"), true
}
