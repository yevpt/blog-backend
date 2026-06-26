// Package dashboard 提供后台首页汇总所需的计数查询。
package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// Counts 是后台首页汇总的原始计数集合。
type Counts struct {
	Articles         int64
	Categories       int64
	Tags             int64
	Music            int64
	FriendLinks      int64
	NewComments      int64
	NewGuestbook     int64
	NewMoments       int64
	UsersTotal       int64
	UsersTodayNew    int64
	UsersTodayActive int64
}

// Repository 汇总各表计数。
type Repository interface {
	// Summary 统计内容总量、自 since 起的新增互动、用户总量与自 dayStart 起的新增/活跃。
	Summary(ctx context.Context, since, dayStart time.Time) (Counts, error)
}

type repository struct{ db *gorm.DB }

// NewRepository 构造首页汇总仓储。
func NewRepository(db *gorm.DB) Repository { return &repository{db: db} }

func (r *repository) Summary(ctx context.Context, since, dayStart time.Time) (Counts, error) {
	db := r.db.WithContext(ctx)
	var c Counts

	count := func(model any, where string, args ...any) (int64, error) {
		var n int64
		q := db.Model(model)
		if where != "" {
			q = q.Where(where, args...)
		}
		if err := q.Count(&n).Error; err != nil {
			return 0, err
		}
		return n, nil
	}

	type job struct {
		dst   *int64
		model any
		where string
		args  []any
	}
	jobs := []job{
		{&c.Articles, &model.Article{}, "", nil},
		{&c.Categories, &model.Category{}, "", nil},
		{&c.Tags, &model.Tag{}, "", nil},
		{&c.Music, &model.Music{}, "", nil},
		{&c.FriendLinks, &model.FriendLink{}, "", nil},
		{&c.NewComments, &model.ArticleComment{}, "created_at >= ?", []any{since}},
		{&c.NewGuestbook, &model.Guestbook{}, "created_at >= ?", []any{since}},
		{&c.NewMoments, &model.Moment{}, "created_at >= ?", []any{since}},
		{&c.UsersTotal, &model.User{}, "", nil},
		{&c.UsersTodayNew, &model.User{}, "created_at >= ?", []any{dayStart}},
		{&c.UsersTodayActive, &model.User{}, "last_active_at >= ?", []any{dayStart}},
	}
	for _, j := range jobs {
		n, err := count(j.model, j.where, j.args...)
		if err != nil {
			return Counts{}, fmt.Errorf("首页汇总计数失败: %w", err)
		}
		*j.dst = n
	}
	return c, nil
}
