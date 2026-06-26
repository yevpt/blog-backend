-- 2026-06-26: 用户最近活跃时间 + 历史数据回填
-- DDL 仅首次部署执行；UPDATE 段幂等（仅补 last_active_at IS NULL 的行）。

ALTER TABLE `user`
  ADD COLUMN `last_active_at` datetime NULL COMMENT '最后活跃时间' AFTER `last_login_at`;

UPDATE `user`
SET `last_active_at` = `last_login_at`
WHERE `last_active_at` IS NULL AND `last_login_at` IS NOT NULL;
