CREATE TABLE IF NOT EXISTS `music_artist` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `name` varchar(100) NOT NULL COMMENT '歌手名',
  `name_zh` varchar(100) DEFAULT NULL COMMENT '中文译名',
  `avatar_key` varchar(500) DEFAULT NULL COMMENT '歌手头像对象 key',
  `description` varchar(500) DEFAULT NULL COMMENT '简介',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_music_artist_name` (`name`),
  KEY `idx_music_artist_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `music_album` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `name` varchar(150) NOT NULL COMMENT '专辑名',
  `artist_id` bigint unsigned DEFAULT NULL COMMENT '主歌手ID',
  `cover_key` varchar(500) DEFAULT NULL COMMENT '专辑封面对象 key',
  `release_date` date DEFAULT NULL COMMENT '发布时间',
  `description` varchar(500) DEFAULT NULL COMMENT '简介',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_music_album_name_artist` (`name`, `artist_id`),
  KEY `idx_music_album_deleted_at` (`deleted_at`),
  KEY `idx_music_album_artist_id` (`artist_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `music_artist_relation` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `music_id` bigint unsigned NOT NULL COMMENT '音乐ID',
  `artist_id` bigint unsigned NOT NULL COMMENT '歌手ID',
  `role` varchar(20) NOT NULL DEFAULT 'primary' COMMENT '角色',
  `seq` int unsigned NOT NULL DEFAULT 0 COMMENT '展示顺序',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_music_artist_relation` (`music_id`, `artist_id`, `role`),
  KEY `idx_music_artist_relation_music_id` (`music_id`),
  KEY `idx_music_artist_relation_artist_id` (`artist_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE `music`
  ADD COLUMN `album_id` bigint unsigned DEFAULT NULL COMMENT '专辑ID' AFTER `album`,
  ADD COLUMN `artist_display_name` varchar(200) NOT NULL DEFAULT '' COMMENT '歌手展示名' AFTER `singer`,
  ADD COLUMN `album_track_no` smallint unsigned NOT NULL DEFAULT 0 COMMENT '专辑序号' AFTER `album_id`,
  ADD COLUMN `audio_key` varchar(500) DEFAULT NULL COMMENT '音频对象 key' AFTER `song_date`,
  ADD COLUMN `audio_size` bigint unsigned NOT NULL DEFAULT 0 COMMENT '音频大小' AFTER `audio_key`,
  ADD COLUMN `audio_mime` varchar(100) NOT NULL DEFAULT '' COMMENT '音频 MIME' AFTER `audio_size`,
  ADD COLUMN `audio_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '音频 hash' AFTER `audio_mime`,
  ADD COLUMN `is_public` tinyint(1) NOT NULL DEFAULT 1 COMMENT '是否公开' AFTER `seq`,
  ADD INDEX `idx_music_album_id` (`album_id`),
  ADD INDEX `idx_music_is_public` (`is_public`),
  ADD INDEX `idx_music_audio_hash` (`audio_hash`);

UPDATE `music`
SET `artist_display_name` = COALESCE(NULLIF(TRIM(`singer`), ''), ''),
    `audio_key` = COALESCE(NULLIF(`audio_key`, ''), NULLIF(`url`, ''))
WHERE `artist_display_name` = ''
   OR `audio_key` IS NULL
   OR `audio_key` = '';

ALTER TABLE `music`
  DROP COLUMN `url`;
