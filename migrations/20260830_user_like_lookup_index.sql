-- 2026-08-30: 优化点赞统计按类型和目标批量聚合查询。
-- 生产现有库仅执行一次；InnoDB 在线 DDL 期间允许并发读写。
ALTER TABLE `user_like`
  ADD INDEX `idx_user_like_type_target_active` (`type`, `target_id`, `deleted_at`),
  ALGORITHM=INPLACE,
  LOCK=NONE;
