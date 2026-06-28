-- 2026-06-27: 内容审核核心表、基础规则与全站控制。
-- 仅包含 Core 阶段表；图片指纹与版本图片快照在后续迁移中创建。

CREATE TABLE IF NOT EXISTS `moderation_item` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `content_type` varchar(40) NOT NULL,
  `content_id` bigint unsigned NOT NULL,
  `author_id` bigint unsigned NOT NULL,
  `lifecycle_state` varchar(16) NOT NULL DEFAULT 'active',
  `public_state` varchar(24) NOT NULL DEFAULT 'placeholder',
  `materialized_revision_id` bigint unsigned NULL,
  `approved_revision_id` bigint unsigned NULL,
  `pending_revision_id` bigint unsigned NULL,
  `state_before_emergency` varchar(24) NULL,
  `emergency_hidden_reason` varchar(1000) NULL,
  `emergency_hidden_at` datetime(3) NULL,
  `deleted_at` datetime(3) NULL,
  `lock_version` bigint unsigned NOT NULL DEFAULT 1,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_moderation_subject` (`content_type`, `content_id`),
  KEY `idx_moderation_item_author` (`author_id`),
  KEY `idx_moderation_item_materialized_revision` (`materialized_revision_id`),
  KEY `idx_moderation_item_approved_revision` (`approved_revision_id`),
  KEY `idx_moderation_item_pending_revision` (`pending_revision_id`),
  KEY `idx_moderation_item_deleted_at` (`deleted_at`),
  CONSTRAINT `chk_moderation_item_content_type` CHECK (`content_type` IN ('moment','article_comment','moment_comment','guestbook','article_comment_reply','moment_comment_reply','guestbook_reply')),
  CONSTRAINT `chk_moderation_item_lifecycle` CHECK (`lifecycle_state` IN ('active','deleted')),
  CONSTRAINT `chk_moderation_item_public_state` CHECK (`public_state` IN ('visible','placeholder','hidden','emergency_hidden')),
  CONSTRAINT `chk_moderation_item_previous_state` CHECK (`state_before_emergency` IS NULL OR `state_before_emergency` IN ('visible','placeholder','hidden'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `moderation_revision` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `item_id` bigint unsigned NOT NULL,
  `version` bigint unsigned NOT NULL,
  `submitter_id` bigint unsigned NOT NULL,
  `idempotency_key` varchar(128) NOT NULL,
  `submitted_content` longtext NOT NULL,
  `published_content` longtext NOT NULL,
  `risk_level` varchar(16) NOT NULL,
  `policy_action` varchar(24) NOT NULL,
  `review_status` varchar(16) NOT NULL,
  `moment_status` tinyint unsigned NULL,
  `moment_comment_status` tinyint unsigned NULL,
  `ruleset_version` bigint unsigned NOT NULL,
  `rule_match_ids` json NOT NULL,
  `decision_type` varchar(24) NULL,
  `decision_reason` varchar(1000) NULL,
  `reviewer_id` bigint unsigned NULL,
  `reviewed_at` datetime(3) NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_moderation_revision_version` (`item_id`, `version`),
  UNIQUE KEY `uk_moderation_revision_idempotency` (`submitter_id`, `idempotency_key`),
  KEY `idx_moderation_revision_item_status` (`item_id`, `review_status`),
  KEY `idx_moderation_revision_submitter` (`submitter_id`),
  KEY `idx_moderation_revision_queue` (`review_status`, `created_at`),
  KEY `idx_moderation_revision_reviewer` (`reviewer_id`),
  CONSTRAINT `fk_moderation_revision_item` FOREIGN KEY (`item_id`) REFERENCES `moderation_item` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `chk_moderation_revision_risk` CHECK (`risk_level` IN ('low','medium','high')),
  CONSTRAINT `chk_moderation_revision_policy` CHECK (`policy_action` IN ('auto_approve','post_review','pre_review','block')),
  CONSTRAINT `chk_moderation_revision_status` CHECK (`review_status` IN ('pending','approved','rejected','superseded')),
  CONSTRAINT `chk_moderation_revision_decision` CHECK (`decision_type` IS NULL OR `decision_type` IN ('approved','corrected','rejected','legacy_migration'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `moderation_attempt` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `content_type` varchar(40) NOT NULL,
  `item_id` bigint unsigned NULL,
  `idempotency_key` varchar(128) NOT NULL,
  `ruleset_version` bigint unsigned NOT NULL,
  `rule_match_ids` json NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_moderation_attempt_idempotency` (`user_id`, `idempotency_key`),
  KEY `idx_moderation_attempt_user_created` (`user_id`, `created_at`),
  KEY `idx_moderation_attempt_item` (`item_id`),
  CONSTRAINT `fk_moderation_attempt_item` FOREIGN KEY (`item_id`) REFERENCES `moderation_item` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `chk_moderation_attempt_content_type` CHECK (`content_type` IN ('moment','article_comment','moment_comment','guestbook','article_comment_reply','moment_comment_reply','guestbook_reply'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `moderation_rule` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL,
  `rule_type` varchar(16) NOT NULL,
  `pattern` varchar(500) NOT NULL,
  `risk_level` varchar(16) NOT NULL,
  `priority` bigint NOT NULL DEFAULT 100,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `ruleset_version` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_moderation_rule_snapshot` (`ruleset_version`, `enabled`, `priority`),
  CONSTRAINT `chk_moderation_rule_type` CHECK (`rule_type` IN ('keyword','regexp','composite')),
  CONSTRAINT `chk_moderation_rule_risk` CHECK (`risk_level` IN ('low','medium','high'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `moderation_action_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `item_id` bigint unsigned NULL,
  `revision_id` bigint unsigned NULL,
  `actor_user_id` bigint unsigned NULL,
  `subject_user_id` bigint unsigned NULL,
  `action` varchar(32) NOT NULL,
  `reason` varchar(1000) NULL,
  `metadata_json` json NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_moderation_action_item_created` (`item_id`, `created_at`),
  KEY `idx_moderation_action_revision` (`revision_id`),
  KEY `idx_moderation_action_actor` (`actor_user_id`),
  KEY `idx_moderation_action_subject_user` (`subject_user_id`),
  CONSTRAINT `fk_moderation_action_log_item` FOREIGN KEY (`item_id`) REFERENCES `moderation_item` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `fk_moderation_action_log_revision` FOREIGN KEY (`revision_id`) REFERENCES `moderation_revision` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `chk_moderation_action` CHECK (`action` IN ('submit','resubmit','approve','correct_and_approve','reject','delete','admin_delete','emergency_hide','restore','trust_change','sanction_change'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `moderation_visible_image` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `item_id` bigint unsigned NOT NULL,
  `revision_id` bigint unsigned NOT NULL,
  `seq` bigint unsigned NOT NULL,
  `object_key` varchar(500) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_moderation_visible_image_seq` (`item_id`, `seq`),
  KEY `idx_moderation_visible_image_revision` (`revision_id`),
  CONSTRAINT `fk_moderation_visible_image_item` FOREIGN KEY (`item_id`) REFERENCES `moderation_item` (`id`) ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT `fk_moderation_visible_image_revision` FOREIGN KEY (`revision_id`) REFERENCES `moderation_revision` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_moderation_profile` (
  `user_id` bigint unsigned NOT NULL,
  `trust_level` varchar(16) NOT NULL DEFAULT 'new',
  `trust_source` varchar(16) NOT NULL DEFAULT 'auto',
  `manual_trust_locked` tinyint(1) NOT NULL DEFAULT 0,
  `sanction_state` varchar(16) NOT NULL DEFAULT 'active',
  `sanction_until` datetime(3) NULL,
  `sanction_reason` varchar(1000) NULL,
  `clean_approval_streak` bigint unsigned NOT NULL DEFAULT 0,
  `corrected_count` bigint unsigned NOT NULL DEFAULT 0,
  `rejected_count` bigint unsigned NOT NULL DEFAULT 0,
  `high_risk_count` bigint unsigned NOT NULL DEFAULT 0,
  `violation_score` bigint NOT NULL DEFAULT 0,
  `last_violation_at` datetime(3) NULL,
  `restricted_until` datetime(3) NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`user_id`),
  KEY `idx_user_moderation_trust` (`trust_level`),
  KEY `idx_user_moderation_sanction` (`sanction_state`, `sanction_until`),
  KEY `idx_user_moderation_restricted_until` (`restricted_until`),
  CONSTRAINT `fk_user_moderation_profile_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT `chk_user_moderation_trust` CHECK (`trust_level` IN ('new','normal','trusted','restricted')),
  CONSTRAINT `chk_user_moderation_trust_source` CHECK (`trust_source` IN ('auto','manual')),
  CONSTRAINT `chk_user_moderation_sanction` CHECK (`sanction_state` IN ('active','muted','banned'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `moderation_control` (
  `id` bigint unsigned NOT NULL,
  `registration_mode` varchar(16) NOT NULL DEFAULT 'open',
  `publishing_mode` varchar(24) NOT NULL DEFAULT 'open',
  `reason` varchar(1000) NULL,
  `operator_id` bigint unsigned NULL,
  `changed_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `lock_version` bigint unsigned NOT NULL DEFAULT 1,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_moderation_control_operator` (`operator_id`),
  CONSTRAINT `fk_moderation_control_operator` FOREIGN KEY (`operator_id`) REFERENCES `user` (`id`) ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT `chk_moderation_control_singleton` CHECK (`id` = 1),
  CONSTRAINT `chk_moderation_registration_mode` CHECK (`registration_mode` IN ('open','closed')),
  CONSTRAINT `chk_moderation_publishing_mode` CHECK (`publishing_mode` IN ('open','pre_review_all','closed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `moderation_rule`
  (`id`, `name`, `rule_type`, `pattern`, `risk_level`, `priority`, `enabled`, `ruleset_version`, `created_at`, `updated_at`)
VALUES
  (1, '礼貌用语基线', 'keyword', '谢谢', 'low', 1000, 1, 1, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)),
  (2, '停用规则示例', 'keyword', '示例停用词', 'medium', 1000, 0, 1, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE `id` = VALUES(`id`);

INSERT INTO `moderation_control`
  (`id`, `registration_mode`, `publishing_mode`, `lock_version`, `created_at`, `updated_at`)
VALUES
  (1, 'open', 'open', 1, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE `id` = VALUES(`id`);
