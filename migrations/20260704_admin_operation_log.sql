-- migrations/20260704_admin_operation_log.sql
-- 2026-07-04: 管理员操作日志表，记录用户管理相关的管理员操作（VIP/账号禁用/审核处罚等）

CREATE TABLE `admin_operation_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `operator_id` bigint unsigned NOT NULL COMMENT '操作人（管理员）用户 ID',
  `target_user_id` bigint unsigned NOT NULL COMMENT '被操作的目标用户 ID',
  `action` varchar(32) NOT NULL COMMENT '操作类型',
  `detail` json NULL COMMENT '操作详情（理由/到期时间等）',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_admin_operation_log_target_user` (`target_user_id`, `created_at`),
  KEY `idx_admin_operation_log_operator` (`operator_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
