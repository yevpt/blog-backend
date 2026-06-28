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
	"regexp"
	"strings"

	"github.com/vpt/blog-backend/pkg/imagefile"
	"github.com/vpt/blog-backend/pkg/storage"
)

var legacyMD5ObjectPattern = regexp.MustCompile(`(?i)^[a-f0-9]{32}\.(?:jpe?g|png|webp|gif)$`)

func (s *service) Prepare(ctx context.Context, userID uint64, objectKeys []string) (PreparedSet, error) {
	if s == nil || s.store == nil || s.registry == nil || userID == 0 {
		return PreparedSet{}, ErrImageUnavailable
	}
	result := PreparedSet{Images: make([]PreparedImage, 0, len(objectKeys)), Replacements: make(map[string]string)}
	cache := make(map[string]PreparedImage, len(objectKeys))
	createdPreviews := make([]string, 0, len(objectKeys))
	cleanup := func() {
		for _, key := range createdPreviews {
			_ = s.store.DeleteObject(ctx, key)
		}
		for _, key := range result.CreatedObjectKeys {
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
			if rawKey != prepared.ObjectKey {
				result.Replacements[rawKey] = prepared.ObjectKey
			}
			continue
		}
		prepared, objectCreated, previewCreated, err := s.prepareOne(ctx, key, userID)
		if err != nil {
			cleanup()
			return PreparedSet{}, err
		}
		if previewCreated {
			createdPreviews = append(createdPreviews, prepared.PreviewObjectKey)
		}
		if objectCreated {
			result.CreatedObjectKeys = append(result.CreatedObjectKeys, prepared.ObjectKey)
		}
		if rawKey != prepared.ObjectKey {
			result.Replacements[rawKey] = prepared.ObjectKey
		}
		if !prepared.Approved {
			err = s.registry.UpsertPendingImage(ctx, PendingImage{
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

func (s *service) prepareOne(ctx context.Context, key string, userID uint64) (PreparedImage, bool, bool, error) {
	data, err := s.store.GetObject(ctx, key)
	if err != nil {
		return PreparedImage{}, false, false, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
	}
	validated, err := imagefile.Validate(path.Base(key), data, int(s.cfg.MaxStoredBytes))
	if err != nil {
		return PreparedImage{}, false, false, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || imageConfig.Width <= 0 || imageConfig.Height <= 0 ||
		int64(imageConfig.Width)*int64(imageConfig.Height) > s.cfg.MaxPixels {
		return PreparedImage{}, false, false, ErrInvalidImage
	}
	shaSum := sha256.Sum256(data)
	md5Sum := md5.Sum(data)
	fingerprint := Fingerprint{
		SHA256: hex.EncodeToString(shaSum[:]), MD5: hex.EncodeToString(md5Sum[:]), Size: uint64(len(data)),
	}
	approved, err := s.registry.UseApprovedImage(ctx, fingerprint, s.now())
	if err != nil {
		return PreparedImage{}, false, false, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
	}
	durableKey, objectCreated, err := s.ensureDurableCommentObject(ctx, key, userID, fingerprint.SHA256, validated.ContentType)
	if err != nil {
		return PreparedImage{}, false, false, err
	}
	prepared := PreparedImage{
		Fingerprint: fingerprint, ObjectKey: durableKey, MediaType: validated.ContentType,
		IsGIF: validated.ContentType == "image/gif", Approved: approved,
	}
	if legacyMD5ObjectPattern.MatchString(key) && !approved {
		return PreparedImage{}, objectCreated, false, ErrInvalidImage
	}
	if approved {
		return prepared, objectCreated, false, nil
	}
	previewKey, created, err := s.preparePreview(ctx, prepared, data)
	if err != nil {
		return PreparedImage{}, objectCreated, false, err
	}
	prepared.PreviewObjectKey = previewKey
	return prepared, objectCreated, created, nil
}

func (s *service) ensureDurableCommentObject(ctx context.Context, key string, userID uint64, sha, mediaType string) (string, bool, error) {
	tempPrefix := fmt.Sprintf("temp/comments/%d/", userID)
	if !strings.HasPrefix(key, tempPrefix) {
		return key, false, nil
	}
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}[mediaType]
	if ext == "" {
		return "", false, ErrInvalidImage
	}
	target := fmt.Sprintf("comments/moderation/%d/%s%s", userID, sha, ext)
	exists, err := s.store.ObjectExists(ctx, target)
	if err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
	}
	if exists {
		return target, false, nil
	}
	if err := s.store.CopyObject(ctx, key, target); err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrImageUnavailable, err)
	}
	return target, true, nil
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
	if !strings.Contains("/"+key, userToken) && !strings.HasPrefix(key, "comments/") &&
		!legacyMD5ObjectPattern.MatchString(key) {
		return "", ErrInvalidImage
	}
	return key, nil
}
