// Package moderation 周期性清理审核审计记录和孤儿对象。
package moderation

import (
	"context"
	"strings"
	"time"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
	"go.uber.org/zap"
)

// ObjectStore 是清理 worker 使用的最小对象存储边界。
type ObjectStore interface {
	storage.ObjectPageLister
	DeleteObject(ctx context.Context, objectName string) error
}

// CleanupResult 汇总一次清理数量。
type CleanupResult struct {
	moderationrepo.AuditCleanupResult
	ImageRecords int
	Objects      int
}

// Worker 按配置的保留期和批次上限执行审核清理。
type Worker struct {
	repo    moderationrepo.CleanupRepository
	store   ObjectStore
	cfg     config.ModerationConfig
	logger  *zap.Logger
	now     func() time.Time
	cursors map[string]string
}

// NewWorker 通过构造注入创建审核清理 worker。
func NewWorker(
	repo moderationrepo.CleanupRepository,
	store ObjectStore,
	cfg config.ModerationConfig,
	logger *zap.Logger,
	now func() time.Time,
) *Worker {
	if logger == nil {
		logger = zap.NewNop()
	}
	if now == nil {
		now = time.Now
	}
	return &Worker{repo: repo, store: store, cfg: cfg, logger: logger, now: now, cursors: make(map[string]string)}
}

// Run 立即执行一次清理，随后按较短配置间隔循环，直到 context 取消。
func (w *Worker) Run(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || w.repo == nil {
		return
	}
	interval := cleanupInterval(w.cfg)
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := w.CleanupOnce(ctx); err != nil {
			w.logger.Warn("审核定期清理失败，等待下次重试", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// CleanupOnce 执行一轮有界数据库和对象清理。
func (w *Worker) CleanupOnce(ctx context.Context) (CleanupResult, error) {
	now := w.now()
	audit, err := w.repo.CleanupAudit(ctx, moderationrepo.AuditCleanupCommand{
		AttemptBefore:   now.AddDate(0, 0, -w.cfg.Audit.AttemptRetentionDays),
		ActionLogBefore: now.AddDate(0, 0, -w.cfg.Audit.ActionLogRetentionDays),
		RevisionBefore:  now.AddDate(0, 0, -w.cfg.Audit.ObsoleteRevisionRetentionDays),
		Limit:           w.cfg.Audit.CleanupBatchSize,
	})
	if err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{AuditCleanupResult: audit}
	result.ImageRecords = w.cleanupStaleImages(ctx, now)
	result.Objects = w.cleanupObjectPrefixes(ctx, now)
	return result, nil
}

func (w *Worker) cleanupStaleImages(ctx context.Context, now time.Time) int {
	cutoff := now.AddDate(0, 0, -w.cfg.Image.ApprovalRetentionDays)
	rows, err := w.repo.ListStaleImages(ctx, cutoff, w.cfg.Image.CleanupBatchSize)
	if err != nil {
		w.logger.Warn("查询过期图片审核记录失败", zap.Error(err))
		return 0
	}
	deleted := 0
	for _, row := range rows {
		if row.PreviewObjectKey != nil && !w.fixedPlaceholder(*row.PreviewObjectKey) {
			if w.store == nil {
				continue
			}
			if err := w.store.DeleteObject(ctx, *row.PreviewObjectKey); err != nil {
				w.logger.Warn("删除过期审核预览失败",
					zap.String("object_key", *row.PreviewObjectKey), zap.Error(err))
				continue
			}
		}
		removed, err := w.repo.DeleteStaleImage(ctx, row.SHA256, row.Size, cutoff)
		if err != nil {
			w.logger.Warn("删除过期图片审核记录失败", zap.String("sha256", row.SHA256), zap.Error(err))
			continue
		}
		if removed {
			deleted++
		}
	}
	return deleted
}

func (w *Worker) cleanupObjectPrefixes(ctx context.Context, now time.Time) int {
	if w.store == nil {
		return 0
	}
	targets := []objectCleanupTarget{
		{prefix: "moderation/previews/", maxAge: w.cfg.Image.OrphanMinAge, checkReference: true},
		{prefix: "temp/", maxAge: w.cfg.Image.TempRetention},
		{prefix: "comments/moderation/", maxAge: w.cfg.Image.OrphanMinAge, checkReference: true},
		{prefix: "moments/", maxAge: w.cfg.Image.OrphanMinAge, checkReference: true, moderationOnly: true},
	}
	deleted := 0
	for _, target := range targets {
		deleted += w.cleanupObjectTarget(ctx, target, now)
	}
	return deleted
}

type objectCleanupTarget struct {
	prefix         string
	maxAge         time.Duration
	checkReference bool
	moderationOnly bool
}

func (w *Worker) cleanupObjectTarget(ctx context.Context, target objectCleanupTarget, now time.Time) int {
	after := w.cursors[target.prefix]
	page, err := w.store.ListObjectPage(ctx, target.prefix, after, w.cfg.Image.CleanupBatchSize)
	if err != nil {
		w.logger.Warn("列出待清理对象失败", zap.String("prefix", target.prefix), zap.Error(err))
		return 0
	}
	if page.HasMore && page.NextAfter != "" {
		w.cursors[target.prefix] = page.NextAfter
	} else {
		w.cursors[target.prefix] = ""
	}
	candidates := oldObjectKeys(page.Objects, target, now)
	if len(candidates) == 0 {
		return 0
	}
	referenced := make(map[string]struct{})
	if target.checkReference {
		referenced, err = w.repo.ReferencedObjectKeys(ctx, candidates)
		if err != nil {
			w.logger.Warn("校验对象引用失败", zap.String("prefix", target.prefix), zap.Error(err))
			return 0
		}
	}
	deleted := 0
	for _, key := range candidates {
		if _, ok := referenced[key]; ok || w.fixedPlaceholder(key) {
			continue
		}
		if err := w.store.DeleteObject(ctx, key); err != nil {
			w.logger.Warn("删除孤儿审核对象失败", zap.String("object_key", key), zap.Error(err))
			continue
		}
		deleted++
	}
	return deleted
}

func oldObjectKeys(objects []storage.ObjectMetadata, target objectCleanupTarget, now time.Time) []string {
	cutoff := now.Add(-target.maxAge)
	keys := make([]string, 0, len(objects))
	for _, object := range objects {
		if object.Key == "" || object.LastModified.IsZero() || object.LastModified.After(cutoff) {
			continue
		}
		if target.moderationOnly && !strings.Contains(object.Key, "/moderation/") {
			continue
		}
		keys = append(keys, object.Key)
	}
	return keys
}

func (w *Worker) fixedPlaceholder(key string) bool {
	return key == w.cfg.Image.StaticPlaceholderKey || key == w.cfg.Image.GIFPlaceholderKey
}

func cleanupInterval(cfg config.ModerationConfig) time.Duration {
	interval := cfg.Image.CleanupInterval
	if interval <= 0 || (cfg.Audit.CleanupInterval > 0 && cfg.Audit.CleanupInterval < interval) {
		interval = cfg.Audit.CleanupInterval
	}
	return interval
}
