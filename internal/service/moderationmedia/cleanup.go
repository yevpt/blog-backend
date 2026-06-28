package moderationmedia

import (
	"context"
	"errors"
	"strings"
)

// DeletePreviewObjects 删除独占预览，固定静态/GIF 占位永不删除。
func (s *service) DeletePreviewObjects(ctx context.Context, keys []string) error {
	if s == nil || s.store == nil {
		return ErrImageUnavailable
	}
	seen := make(map[string]struct{}, len(keys))
	var cleanupErr error
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" || key == s.cfg.StaticPlaceholderKey || key == s.cfg.GIFPlaceholderKey {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := s.store.DeleteObject(ctx, key); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}
