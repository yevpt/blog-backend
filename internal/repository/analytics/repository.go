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
	UpsertDailyDim(ctx context.Context, rows []model.AnalyticsDailyDim) error
	UpsertPageDaily(ctx context.Context, rows []model.AnalyticsPageDaily) error
	DeleteEventsBefore(ctx context.Context, t time.Time) (int64, error)
	DeleteSessionsBefore(ctx context.Context, t time.Time) (int64, error)
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
		DoUpdates: clause.AssignmentColumns([]string{
			"last_seen", "pv_count", "exit_path", "duration", "is_bounce",
			"user_id", "is_authenticated",
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
