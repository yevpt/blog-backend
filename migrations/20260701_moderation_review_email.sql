CREATE TABLE `moderation_review_email_batch` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `recipient_user_id` bigint unsigned NOT NULL,
  `to_email` varchar(155) NOT NULL,
  `subject` varchar(180) NOT NULL,
  `status` varchar(16) NOT NULL,
  `item_count` int NOT NULL DEFAULT 0,
  `scheduled_at` datetime(3) NOT NULL,
  `sent_at` datetime(3) NULL,
  `attempts` int NOT NULL DEFAULT 0,
  `next_attempt_at` datetime(3) NOT NULL,
  `lease_until` datetime(3) NULL,
  `locked_by` varchar(80) NULL,
  `message_id` varchar(120) NULL,
  `last_error` varchar(1000) NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_moderation_review_email_batch_recipient` (`recipient_user_id`),
  KEY `idx_moderation_review_email_batch_pick` (`status`, `next_attempt_at`, `lease_until`),
  CONSTRAINT `fk_moderation_review_email_batch_recipient` FOREIGN KEY (`recipient_user_id`) REFERENCES `user` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `chk_moderation_review_email_batch_status` CHECK (`status` IN ('pending','sending','sent','failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `moderation_review_email_task` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `revision_id` bigint unsigned NOT NULL,
  `item_id` bigint unsigned NOT NULL,
  `status` varchar(16) NOT NULL,
  `available_at` datetime(3) NOT NULL,
  `next_attempt_at` datetime(3) NOT NULL,
  `batch_id` bigint unsigned NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_moderation_review_email_revision` (`revision_id`),
  KEY `idx_moderation_review_email_task_item` (`item_id`),
  KEY `idx_moderation_review_email_task_batch` (`batch_id`),
  KEY `idx_moderation_review_email_task_pick` (`status`, `next_attempt_at`),
  CONSTRAINT `fk_moderation_review_email_task_revision` FOREIGN KEY (`revision_id`) REFERENCES `moderation_revision` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `fk_moderation_review_email_task_item` FOREIGN KEY (`item_id`) REFERENCES `moderation_item` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `fk_moderation_review_email_task_batch` FOREIGN KEY (`batch_id`) REFERENCES `moderation_review_email_batch` (`id`) ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT `chk_moderation_review_email_task_status` CHECK (`status` IN ('pending','batched','sent','skipped'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `moderation_review_email_task`
  (`revision_id`, `item_id`, `status`, `available_at`, `next_attempt_at`, `created_at`, `updated_at`)
SELECT r.`id`, r.`item_id`, 'pending',
       DATE_ADD(CURRENT_TIMESTAMP(3), INTERVAL 60 SECOND),
       DATE_ADD(CURRENT_TIMESTAMP(3), INTERVAL 60 SECOND),
       CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)
FROM `moderation_revision` r
JOIN `moderation_item` i ON i.`pending_revision_id` = r.`id`
WHERE r.`review_status` = 'pending';
