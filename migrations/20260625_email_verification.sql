ALTER TABLE `user`
  ADD COLUMN `email_verified_at` datetime NULL COMMENT '主邮箱验证时间' AFTER `email`;

ALTER TABLE `user_meta`
  ADD COLUMN `sub_email_verified_at` datetime NULL COMMENT '副邮箱验证时间' AFTER `sub_email`;
