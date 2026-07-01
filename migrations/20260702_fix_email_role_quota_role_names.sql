-- 修复 email_role_quota_policy 历史种子数据里的角色名：
-- 早期 seed 代码误写成 admin/normal/vip，与 role 表实际值 ROLE_ADMIN/ROLE_NORMAL/ROLE_VIP 不一致，
-- 导致按角色的邮件配额提升从未生效。UPDATE 按条件过滤，可重复执行。
UPDATE `email_role_quota_policy` SET `role` = 'ROLE_ADMIN' WHERE `role` = 'admin';
UPDATE `email_role_quota_policy` SET `role` = 'ROLE_NORMAL' WHERE `role` = 'normal';
UPDATE `email_role_quota_policy` SET `role` = 'ROLE_VIP' WHERE `role` = 'vip';

-- 补齐 1 号系统管理员（id 固定为 1，见 cmd/dbsetup）的邮箱验证状态：
-- 只在已绑定邮箱但从未标记验证时补验证时间，不影响已有验证记录，可重复执行。
UPDATE `user` SET `email_verified_at` = NOW()
WHERE `id` = 1 AND `email` IS NOT NULL AND `email` <> '' AND `email_verified_at` IS NULL;
