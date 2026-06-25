package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestClassifyReferer(t *testing.T) {
	cases := []struct{ host, site, want string }{
		{"", "yevpt.com", "direct"},
		{"yevpt.com", "yevpt.com", "internal"},
		{"www.google.com", "yevpt.com", "search"},
		{"www.baidu.com", "yevpt.com", "search"},
		{"t.co", "yevpt.com", "social"},
		{"www.zhihu.com", "yevpt.com", "social"},
		{"example.com", "yevpt.com", "external"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, svc.ClassifyReferer(c.host, c.site), c.host)
	}
}
