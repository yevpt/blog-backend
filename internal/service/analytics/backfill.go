package analytics

import (
	"context"
	"fmt"
	"time"
)

// BackfillService 对指定日期区间逐日重算聚合（幂等），用于改规则后重刷历史或补漏天。
type BackfillService interface {
	Backfill(ctx context.Context, from, to string) (days int, err error)
}

type backfillService struct {
	rollupDay func(ctx context.Context, date string) error
}

// NewBackfillService 注入单日聚合函数，避免 service 层依赖 worker 包造成循环依赖。
func NewBackfillService(rollupDay func(ctx context.Context, date string) error) BackfillService {
	return &backfillService{rollupDay: rollupDay}
}

const backfillLayout = "2006-01-02"

// BackfillMaxDays 限制单次回填闭区间最大天数，避免同步重算过多历史数据。
const BackfillMaxDays = 92

// Backfill 逐日（含端点）调用 rollupDay；遇错即停并返回已完成天数。
func (s *backfillService) Backfill(ctx context.Context, from, to string) (int, error) {
	if s.rollupDay == nil {
		return 0, fmt.Errorf("回填函数未配置")
	}
	tz := shanghaiTZ()
	start, err := time.ParseInLocation(backfillLayout, from, tz)
	if err != nil {
		return 0, fmt.Errorf("解析 from 失败: %w", err)
	}
	end, err := time.ParseInLocation(backfillLayout, to, tz)
	if err != nil {
		return 0, fmt.Errorf("解析 to 失败: %w", err)
	}
	if end.Before(start) {
		return 0, fmt.Errorf("to 不能早于 from")
	}
	daysInRange := int(end.Sub(start).Hours()/24) + 1
	if daysInRange > BackfillMaxDays {
		return 0, fmt.Errorf("回填跨度不能超过 %d 天", BackfillMaxDays)
	}

	days := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format(backfillLayout)
		if err := s.rollupDay(ctx, date); err != nil {
			return days, fmt.Errorf("回填 %s 失败: %w", date, err)
		}
		days++
	}
	return days, nil
}
