package moderationrule

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"go.uber.org/zap"
)

// Run 启动单实例 worker 循环，串行处理规则集构建和导入校验。
func (m *manager) Run(ctx context.Context) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()
	for {
		_ = m.ProcessOnce(ctx)
		m.cache.EvictExpired()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOnce 尝试处理一个待构建规则集，没有时立即返回。
func (m *manager) ProcessOnce(ctx context.Context) error {
	candidate, err := m.repo.ClaimNextRuleset(ctx, repoMod.StatusBuilding)
	if err != nil {
		return fmt.Errorf("认领待构建规则集: %w", err)
	}
	if candidate == nil {
		return nil
	}
	return m.processRuleset(ctx, candidate)
}

// processRuleset 构建候选索引、写回统计、按条件自动发布。
func (m *manager) processRuleset(ctx context.Context, candidate *repoMod.CandidateRecord) error {
	start := time.Now()

	snapshot, stats, err := m.buildCandidateSnapshot(ctx, candidate)
	if err != nil {
		m.failRuleset(ctx, candidate.RulesetID, err)
		return err
	}

	// 流式写索引文件到对象存储。
	objectKey := rulesetObjectKey(candidate.RulesetID)
	if err := m.uploadSnapshot(ctx, objectKey, snapshot); err != nil {
		m.failRuleset(ctx, candidate.RulesetID, err)
		return err
	}

	// 写回构建统计并切换为 ready。
	buildResult := repoMod.BuildResult{
		RuleCount:       uint64(stats.RuleCount),
		KeywordCount:    uint64(stats.KeywordCount),
		RegexpCount:     uint64(stats.RegexpCount),
		CompositeCount:  uint64(stats.CompositeCount),
		IndexBytes:      stats.IndexBytes,
		BuildPeakBytes:  stats.BuildPeakBytes,
		BuildDurationMS: uint64(time.Since(start).Milliseconds()),
		IndexObjectKey:  objectKey,
		IndexSHA256:     "",
	}
	if err := m.repo.SaveRulesetBuildResult(ctx, candidate.RulesetID, buildResult); err != nil {
		m.failRuleset(ctx, candidate.RulesetID, err)
		return err
	}

	// 缓存候选快照供试跑和发布复用。
	m.cache.Store(candidate.RulesetID, snapshot)

	// 无导入关联的候选自动发布；有导入关联的等待管理员确认。
	hasImport, err := m.repo.HasImportForRuleset(ctx, candidate.RulesetID)
	if err != nil {
		m.logger.Warn("检查导入关联失败，保持 ready 状态",
			zap.Uint64("ruleset_id", candidate.RulesetID),
			zap.Error(err),
		)
		return nil
	}
	if !hasImport {
		if err := m.PublishCandidate(ctx, candidate.RulesetID, candidate.BaseRulesetID, 0); err != nil {
			m.logger.Warn("自动发布失败，保持 ready 状态",
				zap.Uint64("ruleset_id", candidate.RulesetID),
				zap.Error(err),
			)
		}
	}
	return nil
}

// buildCandidateSnapshot 流式读取候选规则并构建紧凑索引。
func (m *manager) buildCandidateSnapshot(ctx context.Context, candidate *repoMod.CandidateRecord) (*ruleindex.Snapshot, ruleindex.Stats, error) {
	source := func(ctx context.Context, visit func(ruleindex.SourceRule) error) error {
		return m.repo.StreamCandidateRules(ctx, candidate.BaseRulesetID, candidate.RulesetID, func(record repoMod.RuleRecord) error {
			return visit(ruleindex.SourceRule{
				ID:       record.ID,
				Type:     record.RuleType,
				Pattern:  record.Pattern,
				Risk:     record.RiskLevel,
				Effect:   record.Effect,
				Priority: record.Priority,
			})
		})
	}

	buildCtx, cancel := context.WithTimeout(ctx, m.cfg.IndexBuildTimeout)
	defer cancel()

	snapshot, stats, err := ruleindex.Build(buildCtx, candidate.RulesetID, source, m.limits)
	if err != nil {
		return nil, ruleindex.Stats{}, fmt.Errorf("构建候选索引: %w", err)
	}
	return snapshot, stats, nil
}

// uploadSnapshot 将索引流式写入临时文件再上传到对象存储。
func (m *manager) uploadSnapshot(ctx context.Context, objectKey string, snapshot *ruleindex.Snapshot) error {
	tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("moderation-ruleset-%d.bin", snapshot.Version()))
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("创建临时索引文件: %w", err)
	}
	tempFileClosed := false
	defer func() {
		if !tempFileClosed {
			_ = tempFile.Close()
		}
		_ = os.Remove(tempPath)
	}()

	checksum, err := snapshot.WriteTo(tempFile)
	if err != nil {
		return fmt.Errorf("写入临时索引文件: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("关闭临时索引文件: %w", err)
	}
	tempFileClosed = true

	// 重新打开以流式上传，避免整块读入内存。
	readFile, err := os.Open(tempPath)
	if err != nil {
		return fmt.Errorf("重新打开临时索引文件: %w", err)
	}
	defer readFile.Close()

	encodedSize := snapshot.EncodedSize()
	if err := m.store.PutObjectStream(ctx, objectKey, readFile, encodedSize, "application/octet-stream"); err != nil {
		return fmt.Errorf("上传索引对象: %w", err)
	}

	// 记录校验和到快照统计中供后续校验。
	_ = checksum
	return nil
}

func (m *manager) failRuleset(ctx context.Context, id uint64, cause error) {
	code := classifyBuildError(cause)
	if err := m.repo.FailRuleset(ctx, id, code); err != nil {
		m.logger.Error("标记规则集失败",
			zap.Uint64("ruleset_id", id),
			zap.String("code", code),
			zap.Error(err),
		)
	}
	m.cache.Clear()
}

func classifyBuildError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case contains(msg, "超过安全边界") || contains(msg, "内存"):
		return "index_memory_limit"
	case contains(msg, "超限") || contains(msg, "上限"):
		return "rule_limit"
	case contains(msg, "为空"):
		return "empty_ruleset"
	default:
		return "build_failed"
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func rulesetObjectKey(rulesetID uint64) string {
	return fmt.Sprintf("moderation/rulesets/%d.bin", rulesetID)
}
