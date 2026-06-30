CREATE TABLE `moderation_rule_source` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_moderation_rule_source_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `moderation_ruleset` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `base_ruleset_id` bigint unsigned NULL,
  `status` varchar(16) NOT NULL,
  `rule_count` bigint unsigned NOT NULL DEFAULT 0,
  `keyword_count` bigint unsigned NOT NULL DEFAULT 0,
  `regexp_count` bigint unsigned NOT NULL DEFAULT 0,
  `composite_count` bigint unsigned NOT NULL DEFAULT 0,
  `index_bytes` bigint unsigned NOT NULL DEFAULT 0,
  `build_peak_bytes` bigint unsigned NOT NULL DEFAULT 0,
  `build_duration_ms` bigint unsigned NOT NULL DEFAULT 0,
  `index_object_key` varchar(500) NULL,
  `index_format_version` int unsigned NOT NULL DEFAULT 1,
  `index_sha256` char(64) NULL,
  `operator_id` bigint unsigned NULL,
  `failure_code` varchar(64) NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_moderation_ruleset_base` (`base_ruleset_id`),
  KEY `idx_moderation_ruleset_status` (`status`),
  KEY `idx_moderation_ruleset_operator` (`operator_id`),
  CONSTRAINT `fk_moderation_ruleset_base` FOREIGN KEY (`base_ruleset_id`) REFERENCES `moderation_ruleset` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `fk_moderation_ruleset_operator` FOREIGN KEY (`operator_id`) REFERENCES `user` (`id`) ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT `chk_moderation_ruleset_status` CHECK (`status` IN ('building','ready','publishing','published','failed','superseded'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `moderation_ruleset_removal` (
  `ruleset_id` bigint unsigned NOT NULL,
  `rule_id` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`ruleset_id`, `rule_id`),
  KEY `idx_moderation_ruleset_removal_rule` (`rule_id`),
  CONSTRAINT `fk_moderation_ruleset_removal_ruleset` FOREIGN KEY (`ruleset_id`) REFERENCES `moderation_ruleset` (`id`) ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT `fk_moderation_ruleset_removal_rule` FOREIGN KEY (`rule_id`) REFERENCES `moderation_rule` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `moderation_rule_import` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `file_name` varchar(255) NOT NULL,
  `format` varchar(8) NOT NULL,
  `file_size` bigint unsigned NOT NULL,
  `object_key` varchar(500) NOT NULL,
  `source_id` bigint unsigned NOT NULL,
  `default_category` varchar(24) NOT NULL,
  `default_effect` varchar(16) NOT NULL,
  `default_risk_level` varchar(16) NOT NULL,
  `default_priority` int NOT NULL,
  `validation_status` varchar(16) NOT NULL,
  `total_rows` bigint unsigned NOT NULL DEFAULT 0,
  `valid_rows` bigint unsigned NOT NULL DEFAULT 0,
  `duplicate_rows` bigint unsigned NOT NULL DEFAULT 0,
  `error_rows` bigint unsigned NOT NULL DEFAULT 0,
  `error_object_key` varchar(500) NULL,
  `ruleset_id` bigint unsigned NULL,
  `operator_id` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_moderation_rule_import_source` (`source_id`),
  KEY `idx_moderation_rule_import_status` (`validation_status`),
  KEY `idx_moderation_rule_import_ruleset` (`ruleset_id`),
  KEY `idx_moderation_rule_import_operator` (`operator_id`),
  CONSTRAINT `fk_moderation_rule_import_source` FOREIGN KEY (`source_id`) REFERENCES `moderation_rule_source` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `fk_moderation_rule_import_ruleset` FOREIGN KEY (`ruleset_id`) REFERENCES `moderation_ruleset` (`id`) ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT `fk_moderation_rule_import_operator` FOREIGN KEY (`operator_id`) REFERENCES `user` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `chk_moderation_rule_import_format` CHECK (`format` IN ('csv','txt')),
  CONSTRAINT `chk_moderation_rule_import_status` CHECK (`validation_status` IN ('queued','validating','valid','invalid','canceled')),
  CONSTRAINT `chk_moderation_rule_import_category` CHECK (`default_category` IN ('politics','pornography','violence','terrorism','gambling','drugs','fraud','abuse','advertising','minors','other')),
  CONSTRAINT `chk_moderation_rule_import_effect` CHECK (`default_effect` IN ('review','allow')),
  CONSTRAINT `chk_moderation_rule_import_risk` CHECK (`default_risk_level` IN ('low','medium','high'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `moderation_rule_source`
  (`id`, `name`, `created_at`, `updated_at`)
VALUES
  (1, 'system', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3));

INSERT INTO `moderation_ruleset`
  (`id`, `status`, `rule_count`, `keyword_count`, `regexp_count`, `composite_count`, `index_format_version`, `created_at`, `updated_at`)
SELECT
  1,
  'published',
  SUM(CASE WHEN `enabled` = 1 THEN 1 ELSE 0 END),
  SUM(CASE WHEN `enabled` = 1 AND `rule_type` = 'keyword' THEN 1 ELSE 0 END),
  SUM(CASE WHEN `enabled` = 1 AND `rule_type` = 'regexp' THEN 1 ELSE 0 END),
  SUM(CASE WHEN `enabled` = 1 AND `rule_type` = 'composite' THEN 1 ELSE 0 END),
  1,
  CURRENT_TIMESTAMP(3),
  CURRENT_TIMESTAMP(3)
FROM `moderation_rule`;

ALTER TABLE `moderation_rule`
  ADD COLUMN `dedupe_hash` binary(32) NULL,
  ADD COLUMN `category` varchar(24) NOT NULL DEFAULT 'other',
  ADD COLUMN `effect` varchar(16) NOT NULL DEFAULT 'review',
  ADD COLUMN `source_id` bigint unsigned NULL,
  ADD COLUMN `activated_ruleset_id` bigint unsigned NULL,
  ADD COLUMN `deactivated_ruleset_id` bigint unsigned NULL,
  ADD COLUMN `replaces_rule_id` bigint unsigned NULL;

UPDATE `moderation_rule`
SET
  `dedupe_hash` = UNHEX(SHA2(CONCAT('review', CHAR(0), `rule_type`, CHAR(0), `pattern`), 256)),
  `source_id` = 1,
  `activated_ruleset_id` = 1,
  `deactivated_ruleset_id` = CASE WHEN `enabled` = 1 THEN NULL ELSE 1 END;

ALTER TABLE `moderation_rule`
  MODIFY COLUMN `name` varchar(100) NULL,
  MODIFY COLUMN `dedupe_hash` binary(32) NOT NULL,
  MODIFY COLUMN `source_id` bigint unsigned NOT NULL,
  MODIFY COLUMN `activated_ruleset_id` bigint unsigned NOT NULL,
  MODIFY COLUMN `priority` int NOT NULL DEFAULT 100,
  ADD KEY `idx_moderation_rule_dedupe` (`dedupe_hash`),
  ADD KEY `idx_moderation_rule_filter` (`category`, `risk_level`),
  ADD KEY `idx_moderation_rule_source` (`source_id`),
  ADD KEY `idx_moderation_rule_interval` (`activated_ruleset_id`, `deactivated_ruleset_id`),
  ADD KEY `idx_moderation_rule_replaces` (`replaces_rule_id`),
  ADD CONSTRAINT `fk_moderation_rule_source` FOREIGN KEY (`source_id`) REFERENCES `moderation_rule_source` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_moderation_rule_activation` FOREIGN KEY (`activated_ruleset_id`) REFERENCES `moderation_ruleset` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_moderation_rule_deactivation` FOREIGN KEY (`deactivated_ruleset_id`) REFERENCES `moderation_ruleset` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_moderation_rule_replaces` FOREIGN KEY (`replaces_rule_id`) REFERENCES `moderation_rule` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  ADD CONSTRAINT `chk_moderation_rule_category` CHECK (`category` IN ('politics','pornography','violence','terrorism','gambling','drugs','fraud','abuse','advertising','minors','other')),
  ADD CONSTRAINT `chk_moderation_rule_effect` CHECK (`effect` IN ('review','allow'));

ALTER TABLE `moderation_revision`
  ADD COLUMN `rule_matches_truncated` tinyint(1) NOT NULL DEFAULT 0 AFTER `rule_match_ids`;

ALTER TABLE `moderation_attempt`
  ADD COLUMN `rule_matches_truncated` tinyint(1) NOT NULL DEFAULT 0 AFTER `rule_match_ids`;

ALTER TABLE `moderation_rule`
  DROP INDEX `idx_moderation_rule_snapshot`,
  DROP COLUMN `enabled`,
  DROP COLUMN `ruleset_version`;
