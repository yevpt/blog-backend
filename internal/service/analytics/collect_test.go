package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"go.uber.org/zap"
)

type fakeIngestor struct {
	submitted int
	upserts   int
	touches   int
	lastEvent model.AnalyticsEvent
}

func (f *fakeIngestor) Submit(ev model.AnalyticsEvent) bool {
	f.submitted++
	f.lastEvent = ev
	return true
}
func (f *fakeIngestor) UpsertSession(context.Context, model.AnalyticsSession) error {
	f.upserts++
	return nil
}
func (f *fakeIngestor) TouchSession(context.Context, string, time.Time) error {
	f.touches++
	return nil
}

type fakeRT struct {
	online, incr, marks int
	markNew             bool // MarkVisitorSeen 返回值（true = 新访客）
}

func (f *fakeRT) TouchOnline(context.Context, string) error             { f.online++; return nil }
func (f *fakeRT) OnlineCount(context.Context) (int64, error)            { return 0, nil }
func (f *fakeRT) IncrToday(context.Context, model.AnalyticsEvent) error { f.incr++; return nil }
func (f *fakeRT) TodayCounters(context.Context) (svc.TodayStat, error)  { return svc.TodayStat{}, nil }
func (f *fakeRT) MarkVisitorSeen(context.Context, string) (bool, error) {
	f.marks++
	return f.markNew, nil
}

type fakeDedup struct{ dup bool }

func (f *fakeDedup) IsDuplicatePV(context.Context, string, string, string) (bool, error) {
	return f.dup, nil
}

func enr() svc.Enricher { return svc.NewEnricher(fakeGeo{}, "example.com", "salt") }

func TestCollectPageView(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, svc.NewCollectTokenVerifier("", 0, nil), zap.NewNop())
	err := cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Chrome", OriginAllowed: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, ing.submitted)
	assert.Equal(t, 1, ing.upserts)
	assert.Equal(t, 1, rt.incr)
	assert.Equal(t, 1, rt.online)
}

func TestCollectPageViewMarksNewVisitor(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{markNew: true}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, svc.NewCollectTokenVerifier("", 0, nil), zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Chrome", OriginAllowed: true}))
	assert.Equal(t, 1, rt.marks)               // page_view 触发新访客判定
	assert.True(t, ing.lastEvent.IsNewVisitor) // 标记写入入库事件
}

func TestCollectHeartbeatSkipsVisitorMark(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{markNew: true}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, svc.NewCollectTokenVerifier("", 0, nil), zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "heartbeat", VisitorID: "v", SessionID: "s", UA: "Chrome", OriginAllowed: true}))
	assert.Equal(t, 0, rt.marks)      // 心跳不消耗新访客标记
	assert.Equal(t, 0, ing.submitted) // 心跳不入事件表
}

func TestCollectSuspectNotCounted(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, svc.NewCollectTokenVerifier("", 0, nil), zap.NewNop())
	// 伪造/不允许的 Origin → IsSuspect=true（非 bot）。
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Chrome", OriginAllowed: false}))
	assert.Equal(t, 0, rt.online)     // 伪造来源不刷新在线
	assert.Equal(t, 0, rt.incr)       // 伪造来源不计今日
	assert.Equal(t, 1, ing.submitted) // 但仍入库（带 is_suspect 标记，供审计）
	assert.Equal(t, 1, ing.upserts)   // 会话仍 upsert
}

func TestCollectInvalidTokenMarksSuspect(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	// 非空 secret + 非法 token → suspect，不计在线/今日，但仍入库带原因。
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, svc.NewCollectTokenVerifier("secret", 0, nil), zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Chrome", OriginAllowed: true, CollectToken: "bad.token"}))
	assert.Equal(t, 0, rt.online)
	assert.Equal(t, 0, rt.incr)
	assert.Equal(t, 1, ing.submitted)
	assert.True(t, ing.lastEvent.IsSuspect)
	assert.Equal(t, "collect_token_invalid", ing.lastEvent.SuspectReason)
}

func TestCollectDuplicatePVSkipsCount(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: true}, svc.NewCollectTokenVerifier("", 0, nil), zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Chrome", OriginAllowed: true}))
	assert.Equal(t, 0, ing.submitted)
	assert.Equal(t, 0, rt.incr)
	assert.Equal(t, 1, rt.online) // 在线仍刷新
}

func TestCollectBotNotCounted(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, svc.NewCollectTokenVerifier("", 0, nil), zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Googlebot/2.1"}))
	assert.Equal(t, 0, rt.incr)       // bot 不计今日
	assert.Equal(t, 1, ing.submitted) // 但仍入库（带 is_bot 标记）
}

func TestCollectHeartbeat(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, svc.NewCollectTokenVerifier("", 0, nil), zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "heartbeat", VisitorID: "v", SessionID: "s", UA: "Chrome", OriginAllowed: true}))
	assert.Equal(t, 0, ing.submitted) // 心跳不入事件表
	assert.Equal(t, 1, ing.touches)   // 仅刷新会话
	assert.Equal(t, 1, rt.online)
	assert.Equal(t, 0, rt.incr)
}

func TestCollectHeartbeatSkipsTokenCheck(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	// 心跳不校验 collect token：即便携带过期/非法 token 也不应被判 suspect（长会话 token 过期场景）。
	// 对照 TestCollectInvalidTokenMarksSuspect：同样的坏 token 在 page_view 会被判 suspect。
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, svc.NewCollectTokenVerifier("secret", 0, nil), zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "heartbeat", VisitorID: "v", SessionID: "s", UA: "Chrome", OriginAllowed: true, CollectToken: "bad.token"}))
	assert.Equal(t, 1, rt.online)   // 未被判 suspect → 仍刷新在线
	assert.Equal(t, 1, ing.touches) // 心跳更新会话
	assert.Equal(t, 0, ing.submitted)
}
