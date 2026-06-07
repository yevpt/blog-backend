package uv

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTest(t *testing.T) (UVService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { rdb.Close() })
	return NewService(rdb), mr
}

func TestUVService_CheckAndMark_NewVisitor(t *testing.T) {
	t.Parallel()
	svc, _ := setupTest(t)
	ctx := context.Background()

	ok, err := svc.CheckAndMark(ctx, "article:viewed", "123", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("CheckAndMark 返回错误: %v", err)
	}
	if !ok {
		t.Error("首次访问应返回 true")
	}
}

func TestUVService_CheckAndMark_RepeatVisitor(t *testing.T) {
	t.Parallel()
	svc, _ := setupTest(t)
	ctx := context.Background()

	ok, err := svc.CheckAndMark(ctx, "article:viewed", "123", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("第一次 CheckAndMark 返回错误: %v", err)
	}
	if !ok {
		t.Error("首次访问应返回 true")
	}

	ok, err = svc.CheckAndMark(ctx, "article:viewed", "123", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("第二次 CheckAndMark 返回错误: %v", err)
	}
	if ok {
		t.Error("重复访问应返回 false")
	}
}

func TestUVService_CheckAndMark_DifferentVisitors(t *testing.T) {
	t.Parallel()
	svc, _ := setupTest(t)
	ctx := context.Background()

	ok, err := svc.CheckAndMark(ctx, "article:viewed", "123", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("user1 CheckAndMark 返回错误: %v", err)
	}
	if !ok {
		t.Error("user1 首次访问应返回 true")
	}

	ok, err = svc.CheckAndMark(ctx, "article:viewed", "123", "user2", 24*time.Hour)
	if err != nil {
		t.Fatalf("user2 CheckAndMark 返回错误: %v", err)
	}
	if !ok {
		t.Error("不同访客应返回 true")
	}
}

func TestUVService_CheckAndMark_DifferentPrefixes(t *testing.T) {
	t.Parallel()
	svc, _ := setupTest(t)
	ctx := context.Background()

	ok, err := svc.CheckAndMark(ctx, "article:viewed", "123", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("article:viewed CheckAndMark 返回错误: %v", err)
	}
	if !ok {
		t.Error("article:viewed 首次访问应返回 true")
	}

	ok, err = svc.CheckAndMark(ctx, "moment:viewed", "123", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("moment:viewed CheckAndMark 返回错误: %v", err)
	}
	if !ok {
		t.Error("不同业务前缀应返回 true")
	}
}