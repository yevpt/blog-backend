package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestClassifyReferer(t *testing.T) {
	cases := []struct{ host, site, want string }{
		{"", "example.com", "direct"},
		{"example.com", "example.com", "internal"},
		{"www.google.com", "example.com", "search"},
		{"www.baidu.com", "example.com", "search"},
		{"t.co", "example.com", "social"},
		{"www.zhihu.com", "example.com", "social"},
		{"other.com", "example.com", "external"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, svc.ClassifyReferer(c.host, c.site), c.host)
	}
}
