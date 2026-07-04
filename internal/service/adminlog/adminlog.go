package adminlog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	adminlogrepo "github.com/vpt/blog-backend/internal/repository/adminlog"
)

// Action 是操作日志的操作类型枚举。
type Action string

const (
	ActionGrantVIP         Action = "grant_vip"
	ActionRevokeVIP        Action = "revoke_vip"
	ActionDisableAccount   Action = "disable_account"
	ActionEnableAccount    Action = "enable_account"
	ActionMute             Action = "mute"
	ActionBan              Action = "ban"
	ActionRelease          Action = "release"
	ActionUpdateTrustLevel Action = "update_trust_level"
	ActionClearAvatar      Action = "clear_avatar"
)

// Recorder 供各业务 handler 记录一条管理员操作日志。
type Recorder interface {
	Record(ctx context.Context, operatorID, targetUserID uint, action Action, detail map[string]any) error
}

type service struct {
	repo adminlogrepo.Repository
}

// NewService 创建操作日志记录服务。
func NewService(repo adminlogrepo.Repository) Recorder {
	return &service{repo: repo}
}

func (s *service) Record(ctx context.Context, operatorID, targetUserID uint, action Action, detail map[string]any) error {
	entry := &model.AdminOperationLog{
		OperatorID:   operatorID,
		TargetUserID: targetUserID,
		Action:       string(action),
		CreatedAt:    time.Now(),
	}
	if len(detail) > 0 {
		raw, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		text := string(raw)
		entry.Detail = &text
	}
	return s.repo.Create(ctx, entry)
}
