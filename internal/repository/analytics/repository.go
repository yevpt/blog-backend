package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 负责统计原始表写入、聚合表 upsert 与过期清理。
type Repository interface {
	InsertEvents(ctx context.Context, events []model.AnalyticsEvent) error
	UpsertSession(ctx context.Context, s model.AnalyticsSession) error
	TouchSession(ctx context.Context, sessionID string, lastSeen time.Time) error
	UpsertDaily(ctx context.Context, d model.AnalyticsDaily) error
	ReplaceDailyDims(ctx context.Context, date string, rows []model.AnalyticsDailyDim) error
	UpsertDailyDim(ctx context.Context, rows []model.AnalyticsDailyDim) error
	ReplacePageDaily(ctx context.Context, date string, rows []model.AnalyticsPageDaily) error
	UpsertPageDaily(ctx context.Context, rows []model.AnalyticsPageDaily) error
	DeleteEventsBefore(ctx context.Context, t time.Time) (int64, error)
	DeleteSessionsBefore(ctx context.Context, t time.Time) (int64, error)

	QueryDailyRange(ctx context.Context, from, to string) ([]model.AnalyticsDaily, error)
	QueryDimRange(ctx context.Context, dimension, from, to string) ([]model.AnalyticsDailyDim, error)
	QueryTopPages(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error)
	QueryTopPagesPublic(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error)
	QueryTotals(ctx context.Context) (pv, uv int64, err error)
	QueryTotalsSegmented(ctx context.Context) (total, registered, anonymous int64, err error)
	QuerySessionPaths(ctx context.Context, from, to string, limit int) ([]SessionPath, error)
	AggregateDay(ctx context.Context, date string) (DayAggregate, error)
}

// SessionPath 是单个会话按时间排序拼接的访问路径序列，仅含聚合后字段，
// 不含 visitor/user/IP 等可定位个体的列。
type SessionPath struct {
	SessionID string
	Sequence  string
	Steps     int
}

// DayAggregate 是某一日（Asia/Shanghai）从原始事件表聚合出的全量结果，
// 供 rollup worker 落入永久聚合表。类型置于 repo 包以避免 worker→repo 的循环依赖。
type DayAggregate struct {
	Daily model.AnalyticsDaily
	Dims  []model.AnalyticsDailyDim
	Pages []model.AnalyticsPageDaily
}

type repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &repository{db: db} }

func (r *repository) InsertEvents(ctx context.Context, events []model.AnalyticsEvent) error {
	if len(events) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).CreateInBatches(events, 200).Error; err != nil {
		return fmt.Errorf("批量写入事件失败: %w", err)
	}
	return nil
}

func (r *repository) UpsertSession(ctx context.Context, s model.AnalyticsSession) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			// 同一会话再来一次 PV：累计计数、刷新末路径/时间，并据此重算停留与跳出。
			"pv_count":         gorm.Expr("pv_count + 1"),
			"is_bounce":        false, // 出现第二次 PV，不再算跳出
			"last_seen":        s.LastSeen,
			"exit_path":        s.ExitPath,
			"duration":         gorm.Expr("TIMESTAMPDIFF(SECOND, first_seen, ?)", s.LastSeen),
			"user_id":          s.UserID,
			"is_authenticated": s.IsAuthenticated,
		}),
	}).Create(&s).Error
	if err != nil {
		return fmt.Errorf("会话 upsert 失败: %w", err)
	}
	return nil
}

func (r *repository) TouchSession(ctx context.Context, sessionID string, lastSeen time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.AnalyticsSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"last_seen": lastSeen,
			"duration":  gorm.Expr("TIMESTAMPDIFF(SECOND, first_seen, ?)", lastSeen),
		})
	if res.Error != nil {
		return fmt.Errorf("会话心跳更新失败: %w", res.Error)
	}
	return nil
}

func (r *repository) UpsertDaily(ctx context.Context, d model.AnalyticsDaily) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}},
		UpdateAll: true,
	}).Create(&d).Error
	if err != nil {
		return fmt.Errorf("日聚合 upsert 失败: %w", err)
	}
	return nil
}

func (r *repository) ReplaceDailyDims(ctx context.Context, date string, rows []model.AnalyticsDailyDim) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("date = ?", date).Delete(&model.AnalyticsDailyDim{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "date"}, {Name: "dimension"}, {Name: "dim_value"}},
			UpdateAll: true,
		}).CreateInBatches(rows, 200).Error
	})
	if err != nil {
		return fmt.Errorf("替换维度日聚合失败: %w", err)
	}
	return nil
}

func (r *repository) UpsertDailyDim(ctx context.Context, rows []model.AnalyticsDailyDim) error {
	if len(rows) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "dimension"}, {Name: "dim_value"}},
		UpdateAll: true,
	}).CreateInBatches(rows, 200).Error
	if err != nil {
		return fmt.Errorf("维度聚合 upsert 失败: %w", err)
	}
	return nil
}

func (r *repository) ReplacePageDaily(ctx context.Context, date string, rows []model.AnalyticsPageDaily) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("date = ?", date).Delete(&model.AnalyticsPageDaily{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "date"}, {Name: "path"}},
			UpdateAll: true,
		}).CreateInBatches(rows, 200).Error
	})
	if err != nil {
		return fmt.Errorf("替换页面日聚合失败: %w", err)
	}
	return nil
}

func (r *repository) UpsertPageDaily(ctx context.Context, rows []model.AnalyticsPageDaily) error {
	if len(rows) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "path"}},
		UpdateAll: true,
	}).CreateInBatches(rows, 200).Error
	if err != nil {
		return fmt.Errorf("页面聚合 upsert 失败: %w", err)
	}
	return nil
}

func (r *repository) DeleteEventsBefore(ctx context.Context, t time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", t).Delete(&model.AnalyticsEvent{})
	if res.Error != nil {
		return 0, fmt.Errorf("清理过期事件失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

func (r *repository) DeleteSessionsBefore(ctx context.Context, t time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("last_seen < ?", t).Delete(&model.AnalyticsSession{})
	if res.Error != nil {
		return 0, fmt.Errorf("清理过期会话失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}
