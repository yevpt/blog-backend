package moderation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	moderationworker "github.com/vpt/blog-backend/internal/worker/moderation"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type cleanupRepositoryStub struct {
	auditCommand moderationrepo.AuditCleanupCommand
	stale        []moderationrepo.StaleImageRecord
	referenced   map[string]struct{}
	deletedImage []string
}

func (s *cleanupRepositoryStub) CleanupAudit(_ context.Context, cmd moderationrepo.AuditCleanupCommand) (moderationrepo.AuditCleanupResult, error) {
	s.auditCommand = cmd
	return moderationrepo.AuditCleanupResult{Attempts: 1}, nil
}

func (s *cleanupRepositoryStub) ListStaleImages(context.Context, time.Time, int) ([]moderationrepo.StaleImageRecord, error) {
	return s.stale, nil
}

func (s *cleanupRepositoryStub) DeleteStaleImage(_ context.Context, sha string, _ uint64, _ time.Time) (bool, error) {
	s.deletedImage = append(s.deletedImage, sha)
	return true, nil
}

func (s *cleanupRepositoryStub) ReferencedObjectKeys(context.Context, []string) (map[string]struct{}, error) {
	return s.referenced, nil
}

type cleanupStoreStub struct {
	pages   map[string]storage.ObjectPage
	deleted []string
	failKey string
}

func (s *cleanupStoreStub) ListObjectPage(_ context.Context, prefix, _ string, _ int) (storage.ObjectPage, error) {
	return s.pages[prefix], nil
}

func (s *cleanupStoreStub) DeleteObject(_ context.Context, key string) error {
	if key == s.failKey {
		return errors.New("garage unavailable")
	}
	s.deleted = append(s.deleted, key)
	return nil
}

func TestCleanupOnceUsesRetentionProtectsReferencesAndKeepsRunningOnGarageFailure(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	preview := "moderation/previews/stale.jpg"
	fixed := "system/moderation/gif-review.jpg"
	repo := &cleanupRepositoryStub{
		stale: []moderationrepo.StaleImageRecord{
			{SHA256: "stale", Size: 10, PreviewObjectKey: &preview, LastUsedAt: now.AddDate(0, 0, -200)},
			{SHA256: "gif", Size: 11, PreviewObjectKey: &fixed, LastUsedAt: now.AddDate(0, 0, -200)},
		},
		referenced: map[string]struct{}{"moderation/previews/referenced.jpg": {}},
	}
	store := &cleanupStoreStub{
		pages: map[string]storage.ObjectPage{
			"moderation/previews/": {Objects: []storage.ObjectMetadata{
				{Key: "moderation/previews/referenced.jpg", LastModified: now.Add(-48 * time.Hour)},
				{Key: "moderation/previews/orphan.jpg", LastModified: now.Add(-48 * time.Hour)},
				{Key: "moderation/previews/recent.jpg", LastModified: now.Add(-time.Hour)},
			}},
			"temp/": {Objects: []storage.ObjectMetadata{{Key: "temp/old.jpg", LastModified: now.Add(-48 * time.Hour)}}},
		},
		failKey: preview,
	}
	core, logs := observer.New(zap.WarnLevel)
	worker := moderationworker.NewWorker(repo, store, cleanupConfig(), zap.New(core), func() time.Time { return now })

	result, err := worker.CleanupOnce(context.Background())

	require.NoError(t, err)
	assert.Equal(t, now.AddDate(0, 0, -30), repo.auditCommand.AttemptBefore)
	assert.Equal(t, []string{"gif"}, repo.deletedImage)
	assert.ElementsMatch(t, []string{"moderation/previews/orphan.jpg", "temp/old.jpg"}, store.deleted)
	assert.Equal(t, int64(1), result.Attempts)
	assert.NotEmpty(t, logs.All())
}

func cleanupConfig() config.ModerationConfig {
	return config.ModerationConfig{
		Enabled: true,
		Image: config.ModerationImageConfig{
			ApprovalRetentionDays: 180, TempRetention: 24 * time.Hour,
			OrphanMinAge: 24 * time.Hour, CleanupInterval: 24 * time.Hour,
			CleanupBatchSize: 20, StaticPlaceholderKey: "system/moderation/image-review.jpg",
			GIFPlaceholderKey: "system/moderation/gif-review.jpg",
		},
		Audit: config.ModerationAuditConfig{
			AttemptRetentionDays: 30, ActionLogRetentionDays: 365,
			ObsoleteRevisionRetentionDays: 90, CleanupInterval: 24 * time.Hour, CleanupBatchSize: 50,
		},
	}
}
