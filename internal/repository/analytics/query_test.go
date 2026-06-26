package analytics_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryDailyRange(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"date", "pv", "uv"}).
		AddRow("2026-06-23", 10, 5).
		AddRow("2026-06-24", 20, 8)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `analytics_daily`")).
		WillReturnRows(rows)

	got, err := r.QueryDailyRange(context.Background(), "2026-06-23", "2026-06-24")
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, 20, got[1].PV)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryDimRange(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"date", "dimension", "dim_value", "pv", "uv"}).
		AddRow("2026-06-24", "device", "mobile", 12, 7)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `analytics_daily_dim`")).
		WillReturnRows(rows)

	got, err := r.QueryDimRange(context.Background(), "device", "2026-06-23", "2026-06-24")
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, "mobile", got[0].DimValue)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryTopPages(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"path", "title", "pv", "uv"}).
		AddRow("/a", "A", 30, 10).
		AddRow("/b", "B", 20, 6)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT path, max(title) as title, sum(pv) as pv, sum(uv) as uv FROM `analytics_page_daily`")).
		WillReturnRows(rows)

	got, err := r.QueryTopPages(context.Background(), "2026-06-23", "2026-06-24", 5)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, "/a", got[0].Path)
	assert.Equal(t, 30, got[0].PV)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryTotals(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"pv", "uv"}).AddRow(int64(100), int64(40))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(pv),0) as pv, COALESCE(SUM(uv),0) as uv FROM `analytics_daily`")).
		WillReturnRows(rows)

	pv, uv, err := r.QueryTotals(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(100), pv)
	assert.Equal(t, int64(40), uv)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryTopPagesPublic_ExcludesAdmin(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"path", "title", "pv", "uv"}).AddRow("/a", "A", 30, 10)
	// 必须带 path NOT LIKE 排除 /admin/*
	mock.ExpectQuery("analytics_page_daily.*NOT LIKE").WillReturnRows(rows)
	got, err := r.QueryTopPagesPublic(context.Background(), "2026-06-01", "2026-06-30", 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "/a", got[0].Path)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryTotalsSegmented(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"total", "registered", "anonymous"}).
		AddRow(int64(40), int64(10), int64(30))
	mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_daily`")).WillReturnRows(rows)
	total, reg, anon, err := r.QueryTotalsSegmented(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(40), total)
	assert.Equal(t, int64(10), reg)
	assert.Equal(t, int64(30), anon)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQuerySessionPaths(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"session_id", "sequence", "steps"}).
		AddRow("s1", "/,/articles,/articles/1", 3)
	mock.ExpectQuery("GROUP_CONCAT\\(path ORDER BY created_at SEPARATOR ','\\).*FROM `analytics_events`").
		WillReturnRows(rows)
	got, err := r.QuerySessionPaths(context.Background(), "2026-06-01", "2026-06-02", 1000)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "/,/articles,/articles/1", got[0].Sequence)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryRecentActivePaths(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"path", "active"}).
		AddRow("/articles", 5).
		AddRow("/", 3)
	// 须命中事件表、按 path 分组、按 active 降序并限幅；eventScope 带 is_bot/is_suspect 过滤。
	mock.ExpectQuery("COUNT\\(DISTINCT visitor_id\\).*FROM `analytics_events`.*is_bot.*is_suspect.*GROUP BY.*`path`.*ORDER BY active DESC.*LIMIT").
		WillReturnRows(rows)

	got, err := r.QueryRecentActivePaths(context.Background(), time.Now().Add(-5*time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "/articles", got[0].Path)
	assert.Equal(t, 5, got[0].Active)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAggregateDay 验证 AggregateDay 依次发出 Daily/各维度/Page 聚合查询并组装结果。
func TestAggregateDay(t *testing.T) {
	r, mock := newRepo(t)

	// Daily 聚合（首条查询）
	dailyCols := []string{"pv", "uv", "registered_pv", "registered_uv", "anonymous_pv", "anonymous_uv", "sessions", "new_visitors"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_events`")).
		WillReturnRows(sqlmock.NewRows(dailyCols).AddRow(50, 20, 30, 8, 20, 12, 15, 6))

	// 6 个维度，每个一条 GROUP BY 查询
	dimCols := []string{"dim_value", "pv", "uv"}
	for i := 0; i < 6; i++ {
		mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_events`")).
			WillReturnRows(sqlmock.NewRows(dimCols).AddRow("val", 5, 3))
	}

	// Page 聚合
	pageCols := []string{"path", "title", "pv", "uv"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_events`")).
		WillReturnRows(sqlmock.NewRows(pageCols).AddRow("/x", "X", 9, 4))

	// 友链来源聚合：按已配置友链站点 host 匹配 referer_host。
	friendCols := []string{"friend_link_id", "friend_name", "site", "site_host", "pv", "uv", "sessions"}
	mock.ExpectQuery("FROM analytics_events AS e.*friend_link AS f").
		WillReturnRows(sqlmock.NewRows(friendCols).AddRow(uint(7), "友站", "https://friend.example.com", "friend.example.com", 11, 5, 4))

	// 会话级指标查询（最后一条）；须排除 suspect 会话（is_suspect=false）。
	mock.ExpectQuery("FROM `analytics_sessions`.*is_suspect").
		WillReturnRows(sqlmock.NewRows([]string{"avg_duration", "bounce_rate"}).AddRow(42.0, 0.25))

	got, err := r.AggregateDay(context.Background(), "2026-06-24")
	require.NoError(t, err)
	assert.Equal(t, "2026-06-24", got.Daily.Date)
	assert.Equal(t, 50, got.Daily.PV)
	assert.Equal(t, 20, got.Daily.UV)
	assert.Equal(t, 30, got.Daily.RegisteredPV)
	assert.Equal(t, 15, got.Daily.Sessions)
	// 每个维度一行，共 6 行
	assert.Len(t, got.Dims, 6)
	assert.Equal(t, "2026-06-24", got.Dims[0].Date)
	assert.Len(t, got.Pages, 1)
	assert.Equal(t, "/x", got.Pages[0].Path)
	assert.Equal(t, "2026-06-24", got.Pages[0].Date)
	require.Len(t, got.FriendLinks, 1)
	assert.Equal(t, uint(7), got.FriendLinks[0].FriendLinkID)
	assert.Equal(t, "friend.example.com", got.FriendLinks[0].SiteHost)
	assert.Equal(t, 11, got.FriendLinks[0].PV)
	assert.Equal(t, 42, got.Daily.AvgDuration)
	assert.Equal(t, 0.25, got.Daily.BounceRate)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAggregateDay_EmptyEventDay(t *testing.T) {
	r, mock := newRepo(t)

	dailyCols := []string{"pv", "uv", "registered_pv", "registered_uv", "anonymous_pv", "anonymous_uv", "sessions", "new_visitors"}
	mock.ExpectQuery("COALESCE\\(SUM\\(is_authenticated\\), ?0\\).*COALESCE\\(SUM\\(NOT is_authenticated\\), ?0\\).*FROM `analytics_events`").
		WillReturnRows(sqlmock.NewRows(dailyCols).AddRow(0, 0, 0, 0, 0, 0, 0, 0))

	dimCols := []string{"dim_value", "pv", "uv"}
	for i := 0; i < 6; i++ {
		mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_events`")).
			WillReturnRows(sqlmock.NewRows(dimCols))
	}

	pageCols := []string{"path", "title", "pv", "uv"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_events`")).
		WillReturnRows(sqlmock.NewRows(pageCols))

	friendCols := []string{"friend_link_id", "friend_name", "site", "site_host", "pv", "uv", "sessions"}
	mock.ExpectQuery("FROM analytics_events AS e.*friend_link AS f").
		WillReturnRows(sqlmock.NewRows(friendCols))

	mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_sessions`")).
		WillReturnRows(sqlmock.NewRows([]string{"avg_duration", "bounce_rate"}).AddRow(0.0, 0.0))

	got, err := r.AggregateDay(context.Background(), "2026-06-24")
	require.NoError(t, err)
	assert.Equal(t, "2026-06-24", got.Daily.Date)
	assert.Zero(t, got.Daily.PV)
	assert.Empty(t, got.Dims)
	assert.Empty(t, got.Pages)
	assert.Empty(t, got.FriendLinks)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryFriendLinkDaily(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"friend_link_id", "friend_name", "site", "site_host", "pv", "uv", "sessions"}).
		AddRow(uint(7), "友站", "https://friend.example.com", "friend.example.com", 12, 6, 4)
	mock.ExpectQuery("FROM `analytics_friend_link_daily`.*GROUP BY.*friend_link_id").
		WillReturnRows(rows)

	got, err := r.QueryFriendLinkDaily(context.Background(), "2026-06-01", "2026-06-30", 20)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, uint(7), got[0].FriendLinkID)
	assert.Equal(t, "友站", got[0].FriendName)
	assert.Equal(t, 12, got[0].PV)
	assert.Equal(t, 4, got[0].Sessions)
	require.NoError(t, mock.ExpectationsWereMet())
}
