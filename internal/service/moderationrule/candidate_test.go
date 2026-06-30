package moderationrule

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
	repoMock "github.com/vpt/blog-backend/internal/repository/moderationrule/mock"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
)

func TestReplaceRuleCreatesNewFactAndRemovalCandidate(t *testing.T) {
	repo, mgr := newTestManager(t)
	repo.EXPECT().CurrentStatus(gomock.Any()).Return(repoMod.StatusRecord{CurrentRulesetID: 7}, nil)
	repo.EXPECT().FindDuplicateHashes(gomock.Any(), uint64(7), gomock.Any()).Return(nil, nil)
	repo.EXPECT().CreateCandidate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, cmd repoMod.CreateCandidateCommand) (repoMod.CandidateRecord, error) {
		assert.Equal(t, uint64(7), cmd.BaseRulesetID)
		assert.Equal(t, []uint64{41}, cmd.RemoveRuleIDs)
		require.Len(t, cmd.Additions, 1)
		assert.Equal(t, "风险词", cmd.Additions[0].Pattern)
		return repoMod.CandidateRecord{RulesetID: 8, BaseRulesetID: 7, Status: "building"}, nil
	})

	input := RuleInput{RuleType: "keyword", Pattern: "风险词", Category: "other", Effect: "review", RiskLevel: "medium", Priority: 100, SourceID: 1}
	got, err := mgr.ReplaceRule(context.Background(), ReplaceRuleCommand{RuleID: 41, ExpectedRulesetID: 7, ActorID: 1, Rule: input})

	require.NoError(t, err)
	assert.Equal(t, uint64(8), got.RulesetID)
	assert.Equal(t, uint64(7), got.BaseRulesetID)
}

func TestCreateRuleRejectsStaleExpectedVersion(t *testing.T) {
	repo, mgr := newTestManager(t)
	repo.EXPECT().CurrentStatus(gomock.Any()).Return(repoMod.StatusRecord{CurrentRulesetID: 7}, nil)

	input := RuleInput{RuleType: "keyword", Pattern: "新词", Category: "other", Effect: "review", RiskLevel: "medium", Priority: 100, SourceID: 1}
	_, err := mgr.CreateRule(context.Background(), CreateRuleCommand{ExpectedRulesetID: 6, ActorID: 1, Rule: input})

	assert.ErrorIs(t, err, ErrRulesetConflict)
}

func TestCreateRuleRejectsInvalidCategory(t *testing.T) {
	_, mgr := newTestManager(t)

	input := RuleInput{RuleType: "keyword", Pattern: "新词", Category: "invalid_cat", Effect: "review", RiskLevel: "medium", Priority: 100, SourceID: 1}
	_, err := mgr.CreateRule(context.Background(), CreateRuleCommand{ExpectedRulesetID: 7, ActorID: 1, Rule: input})

	assert.ErrorIs(t, err, ErrInvalidRule)
}

func TestCreateRuleRejectsAllowForNonKeyword(t *testing.T) {
	_, mgr := newTestManager(t)

	input := RuleInput{RuleType: "regexp", Pattern: "test.*", Category: "other", Effect: "allow", RiskLevel: "low", Priority: 100, SourceID: 1}
	_, err := mgr.CreateRule(context.Background(), CreateRuleCommand{ExpectedRulesetID: 7, ActorID: 1, Rule: input})

	assert.ErrorIs(t, err, ErrInvalidRule)
}

func TestCreateRuleRejectsDuplicate(t *testing.T) {
	repo, mgr := newTestManager(t)
	repo.EXPECT().CurrentStatus(gomock.Any()).Return(repoMod.StatusRecord{CurrentRulesetID: 7}, nil)
	hash, _ := computeDedupeHash("review", "keyword", "风险词")
	dupeMap := map[repoMod.DedupeHash]uint64{hash: 42}
	repo.EXPECT().FindDuplicateHashes(gomock.Any(), uint64(7), gomock.Any()).Return(dupeMap, nil)

	input := RuleInput{RuleType: "keyword", Pattern: "风险词", Category: "other", Effect: "review", RiskLevel: "medium", Priority: 100, SourceID: 1}
	_, err := mgr.CreateRule(context.Background(), CreateRuleCommand{ExpectedRulesetID: 7, ActorID: 1, Rule: input})

	assert.ErrorIs(t, err, ErrDuplicateRule)
}

func TestPublishRejectsStaleBaseWithoutReplacingSnapshot(t *testing.T) {
	repo, mgr, replacer := newTestManagerWithReplacer(t)
	const candidateID uint64 = 8
	repo.EXPECT().PublishCandidate(gomock.Any(), candidateID, uint64(7)).Return(repoMod.ErrRulesetConflict)

	err := mgr.PublishCandidate(context.Background(), candidateID, 7, 1)

	assert.ErrorIs(t, err, ErrRulesetConflict)
	assert.Equal(t, 0, replacer.replaceCalls)
}

func TestCancelCandidateClearsCache(t *testing.T) {
	repo, mgr := newTestManager(t)
	repo.EXPECT().CancelCandidate(gomock.Any(), uint64(8), uint64(1)).Return(nil)

	err := mgr.CancelCandidate(context.Background(), 8, 1)

	require.NoError(t, err)
}

func TestCandidateCacheEvictsOnTTLAndCancel(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	cache := newCandidateCache(10*time.Minute, clock.Now)
	snapshot := buildTestSnapshot(t)
	cache.Store(8, snapshot)
	assert.NotNil(t, cache.Load(8))

	clock.Advance(11 * time.Minute)
	cache.EvictExpired()
	assert.Nil(t, cache.Load(8))
}

func TestCandidateCacheClearReleasesImmediately(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	cache := newCandidateCache(10*time.Minute, clock.Now)
	snapshot := buildTestSnapshot(t)
	cache.Store(8, snapshot)
	assert.NotNil(t, cache.Load(8))

	cache.Clear()
	assert.Nil(t, cache.Load(8))
	assert.Equal(t, uint64(0), cache.CurrentRulesetID())
}

func TestCandidateCacheRejectsWrongRulesetID(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	cache := newCandidateCache(10*time.Minute, clock.Now)
	snapshot := buildTestSnapshot(t)
	cache.Store(8, snapshot)

	assert.Nil(t, cache.Load(9))
	assert.NotNil(t, cache.Load(8))
}

func buildTestSnapshot(t *testing.T) *ruleindex.Snapshot {
	t.Helper()
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "测试", Risk: "medium", Effect: "review"},
	}
	source := func(_ context.Context, visit func(ruleindex.SourceRule) error) error {
		for _, rule := range rules {
			if err := visit(rule); err != nil {
				return err
			}
		}
		return nil
	}
	snapshot, _, err := ruleindex.Build(context.Background(), 1, source, ruleindex.Limits{
		MaxKeywordRules: 500000, MaxRegexpRules: 200, MaxPatternRunes: 500, MaxMatchIDs: 128,
	})
	require.NoError(t, err)
	return snapshot
}

type fakeReplacer struct {
	mu           sync.Mutex
	replaceCalls int
}

func (f *fakeReplacer) ReplaceSnapshot(_ *ruleindex.Snapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replaceCalls++
	return nil
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestManager(t *testing.T) (*repoMock.MockManagementRepository, Service) {
	t.Helper()
	repo, mgr, _ := newTestManagerWithReplacer(t)
	return repo, mgr
}

func newTestManagerWithReplacer(t *testing.T) (*repoMock.MockManagementRepository, Service, *fakeReplacer) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := repoMock.NewMockManagementRepository(ctrl)
	replacer := &fakeReplacer{}
	cfg := ManagerConfig{
		MaxPatternChars:      500,
		MaxKeywordRules:      500000,
		MaxEnabledRegexRules: 200,
		MaxRuleMatches:       128,
		MaxIndexMemoryMB:     512,
		MaxBuildPeakMemoryMB: 1024,
		IndexBuildTimeout:    10 * time.Minute,
		CandidateCacheTTL:    10 * time.Minute,
	}
	svc := NewManager(repo, nil, replacer, cfg, nil)
	return repo, svc, replacer
}
