package moderationmedia

import (
	"bytes"
	"context"
	"fmt"

	"github.com/vpt/blog-backend/pkg/imageutil"
)

func (s *service) preparePreview(ctx context.Context, image PreparedImage, data []byte) (string, bool, error) {
	if image.IsGIF {
		return s.cfg.GIFPlaceholderKey, false, nil
	}
	result, err := imageutil.Process(bytes.NewReader(data), imageutil.Options{
		MaxWidth: s.cfg.PreviewMaxEdge, MaxHeight: s.cfg.PreviewMaxEdge,
		Format: imageutil.FormatWebP, WebPQuality: 60, MinWebPQuality: 40,
	})
	if err != nil {
		return s.cfg.StaticPlaceholderKey, false, nil
	}
	key := fmt.Sprintf("moderation/previews/%s/%s.webp", image.SHA256[:2], image.SHA256)
	exists, err := s.store.ObjectExists(ctx, key)
	if err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
	}
	if exists {
		return key, false, nil
	}
	if err := s.store.PutObject(ctx, key, result.Bytes, result.ContentType); err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
	}
	return key, true, nil
}
