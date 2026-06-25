package analytics

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/service/uv"
)

const pvDedupWindow = 5 * time.Second

type uvDedup struct{ uv uv.UVService }

// NewDedupChecker 用已有 UV 去重服务实现短窗口 PV 去重。
func NewDedupChecker(u uv.UVService) DedupChecker { return &uvDedup{uv: u} }

// IsDuplicatePV 借 UV 服务标记「会话+路径」短窗口访问，非新即重复。
func (d *uvDedup) IsDuplicatePV(ctx context.Context, visitorID, sessionID, path string) (bool, error) {
	isNew, err := d.uv.CheckAndMark(ctx, "analytics:pv:dedup", sessionID+"|"+path, visitorID, pvDedupWindow)
	if err != nil {
		return false, err
	}
	return !isNew, nil // 非新 = 重复
}
