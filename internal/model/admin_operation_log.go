package model

import "time"

// AdminOperationLog 管理员对用户执行管理操作的审计记录。
type AdminOperationLog struct {
	ID           uint64    `gorm:"primarykey" json:"id"`
	OperatorID   uint      `gorm:"not null;index:idx_admin_operation_log_operator,priority:1" json:"operator_id"`
	TargetUserID uint      `gorm:"not null;index:idx_admin_operation_log_target_user,priority:1" json:"target_user_id"`
	Action       string    `gorm:"size:32;not null" json:"action"`
	Detail       *string   `gorm:"type:json" json:"detail,omitempty"`
	CreatedAt    time.Time `gorm:"type:datetime(3);not null" json:"created_at"`
}

func (AdminOperationLog) TableName() string { return "admin_operation_log" }
