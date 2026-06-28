package moderationmedia

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"path"
	"strings"

	"github.com/vpt/blog-backend/pkg/imagefile"
	"github.com/vpt/blog-backend/pkg/storage"
)

func (s *service) Prepare(ctx context.Context, userID uint64, objectKeys []string) (PreparedSet, error) {
	if s == nil || s.store == nil || s.registry == nil || userID == 0 {
		return PreparedSet{}, ErrImageUnavailable
	}
	result := PreparedSet{Images: make([]PreparedImage, 0, len(objectKeys))}
	cache := make(map[string]PreparedImage, len(objectKeys))
	createdPreviews := make([]string, 0, len(objectKeys))
	cleanup := func() {
		for _, key := range createdPreviews {
			_ = s.store.DeleteObject(ctx, key)
		}
	}

	for _, rawKey := range objectKeys {
		key, err := s.safeObjectKey(rawKey, userID)
		if err != nil {
			cleanup()
			return PreparedSet{}, err
		}
		if prepared, ok := cache[key]; ok {
			result.Images = append(result.Images, prepared)
			continue
		}
		prepared, previewCreated, err := s.prepareOne(ctx, key)
		if err != nil {
			cleanup()
			return PreparedSet{}, err
		}
		if previewCreated {
			createdPreviews = append(createdPreviews, prepared.PreviewObjectKey)
		}
		if !prepared.Approved {
			err = s.registry.UpsertPending(ctx, PendingImage{
				Fingerprint: prepared.Fingerprint, PreviewObjectKey: prepared.PreviewObjectKey, LastUsedAt: s.now(),
			})
			if err != nil {
				cleanup()
				return PreparedSet{}, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
			}
		}
		cache[key] = prepared
		result.Images = append(result.Images, prepared)
	}
	return result, nil
}

func (s *service) prepareOne(ctx context.Context, key string) (PreparedImage, bool, error) {
	data, err := s.store.GetObject(ctx, key)
	if err != nil {
		return PreparedImage{}, false, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
	}
	validated, err := imagefile.Validate(path.Base(key), data, int(s.cfg.MaxStoredBytes))
	if err != nil {
		return PreparedImage{}, false, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || imageConfig.Width <= 0 || imageConfig.Height <= 0 ||
		int64(imageConfig.Width)*int64(imageConfig.Height) > s.cfg.MaxPixels {
		return PreparedImage{}, false, ErrInvalidImage
	}
	shaSum := sha256.Sum256(data)
	md5Sum := md5.Sum(data)
	fingerprint := Fingerprint{
		SHA256: hex.EncodeToString(shaSum[:]), MD5: hex.EncodeToString(md5Sum[:]), Size: uint64(len(data)),
	}
	approved, err := s.registry.UseApproved(ctx, fingerprint, s.now())
	if err != nil {
		return PreparedImage{}, false, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
	}
	prepared := PreparedImage{
		Fingerprint: fingerprint, ObjectKey: key, MediaType: validated.ContentType,
		IsGIF: validated.ContentType == "image/gif", Approved: approved,
	}
	if approved {
		return prepared, false, nil
	}
	previewKey, created, err := s.preparePreview(ctx, prepared, data)
	if err != nil {
		return PreparedImage{}, false, err
	}
	prepared.PreviewObjectKey = previewKey
	return prepared, created, nil
}

func (s *service) safeObjectKey(value string, userID uint64) (string, error) {
	key, err := s.store.ObjectKey(value)
	if err != nil {
		if errors.Is(err, storage.ErrExternalObjectURL) {
			return "", ErrInvalidImage
		}
		return "", fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key == "" || strings.Contains(key, "..") || storage.IsAbsoluteURL(key) {
		return "", ErrInvalidImage
	}
	userToken := fmt.Sprintf("/%d/", userID)
	if !strings.Contains("/"+key, userToken) && !strings.HasPrefix(key, "comments/") {
		return "", ErrInvalidImage
	}
	return key, nil
}
