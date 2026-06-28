-- 文章新增移动端封面字段
ALTER TABLE `article`
    ADD COLUMN `mobile_cover_img_url` varchar(500) DEFAULT NULL COMMENT '移动端封面图URL' AFTER `cover_img_url`;
