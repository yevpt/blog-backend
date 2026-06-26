package middleware

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

// 指向一个必定连不上的地址，确保 Incr 必定出错而不是真的连上 Redis
func newUnreachableRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  0,
	})
}

func TestGetEscalatedBanDuration_FallsBackOnRedisError(t *testing.T) {
	rdb := newUnreachableRedis()
	defer rdb.Close()

	got := getEscalatedBanDuration(rdb, "ban:test:count")

	assert.Equal(t, escalatingBanDurations[0], got)
}

func TestGetEscalatedBanDurationPublic_FallsBackOnRedisError(t *testing.T) {
	rdb := newUnreachableRedis()
	defer rdb.Close()

	got := getEscalatedBanDurationPublic(rdb, "ban:public:test:count")

	assert.Equal(t, escalatingBanDurationsPublic[0], got)
}
