package moderationrule

import (
	"sync"
	"time"

	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
)

// candidateCache 最多持有一个候选快照，TTL 到期或显式清除后释放引用。
// 设计约束：handler、任务闭包和独立 timer 不得长期持有候选；
// TTL 由 worker 的单一周期检查完成，大型切片不进入 sync.Pool。
type candidateCache struct {
	mu       sync.Mutex
	ttl      time.Duration
	now      func() time.Time
	entry    *candidateEntry
}

type candidateEntry struct {
	rulesetID uint64
	snapshot  *ruleindex.Snapshot
	storedAt  time.Time
}

// newCandidateCache 创建有界候选缓存，ttl 为零值时永不自动过期。
func newCandidateCache(ttl time.Duration, now func() time.Time) *candidateCache {
	if now == nil {
		now = time.Now
	}
	return &candidateCache{ttl: ttl, now: now}
}

// Store 写入候选快照，覆盖之前的引用。
func (c *candidateCache) Store(rulesetID uint64, snapshot *ruleindex.Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entry = &candidateEntry{
		rulesetID: rulesetID,
		snapshot:  snapshot,
		storedAt:  c.now(),
	}
}

// Load 返回未过期的候选快照，过期或不存在时返回 nil。
func (c *candidateCache) Load(rulesetID uint64) *ruleindex.Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entry == nil || c.entry.rulesetID != rulesetID {
		return nil
	}
	if c.ttl > 0 && c.now().Sub(c.entry.storedAt) >= c.ttl {
		c.entry = nil
		return nil
	}
	return c.entry.snapshot
}

// EvictExpired 清除过期引用，由 worker 周期调用。
func (c *candidateCache) EvictExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entry == nil {
		return
	}
	if c.ttl > 0 && c.now().Sub(c.entry.storedAt) >= c.ttl {
		c.entry = nil
	}
}

// Clear 立即清除候选引用，用于发布、取消或失败后。
func (c *candidateCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entry = nil
}

// CurrentRulesetID 返回当前缓存的规则集 ID，没有时返回 0。
func (c *candidateCache) CurrentRulesetID() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entry == nil {
		return 0
	}
	return c.entry.rulesetID
}
