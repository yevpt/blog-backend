-- 2026-06-29: 内容审核图片版本快照与全站审核记录。

CREATE TABLE IF NOT EXISTS `moderation_revision_image` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `revision_id` bigint unsigned NOT NULL,
  `seq` bigint unsigned NOT NULL,
  `object_key` varchar(500) NOT NULL,
  `sha256` varchar(64) NOT NULL,
  `md5` varchar(32) NOT NULL,
  `size` bigint unsigned NOT NULL,
  `media_type` varchar(100) NOT NULL,
  `is_gif` tinyint(1) NOT NULL DEFAULT 0,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_moderation_revision_image_seq` (`revision_id`, `seq`),
  KEY `idx_moderation_revision_image_sha` (`revision_id`, `sha256`),
  KEY `idx_moderation_revision_image_md5` (`md5`),
  CONSTRAINT `fk_moderation_revision_image_revision` FOREIGN KEY (`revision_id`) REFERENCES `moderation_revision` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `moderation_image` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `sha256` varchar(64) NOT NULL,
  `size` bigint unsigned NOT NULL,
  `md5` varchar(32) NOT NULL,
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `preview_object_key` varchar(500) NULL,
  `approved_at` datetime(3) NULL,
  `approved_by` bigint unsigned NULL,
  `last_used_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_moderation_image_fingerprint` (`sha256`, `size`),
  KEY `idx_moderation_image_md5` (`md5`),
  KEY `idx_moderation_image_status` (`status`),
  KEY `idx_moderation_image_approved_by` (`approved_by`),
  KEY `idx_moderation_image_last_used_at` (`last_used_at`),
  CONSTRAINT `fk_moderation_image_approved_by` FOREIGN KEY (`approved_by`) REFERENCES `user` (`id`) ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT `chk_moderation_image_status` CHECK (`status` IN ('pending','approved'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
