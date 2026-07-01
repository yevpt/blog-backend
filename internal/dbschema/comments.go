package dbschema

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// SchemaCommentSet 保存数据库表与字段注释。
type SchemaCommentSet struct {
	Tables map[string]TableComment
}

// TableComment 保存单张表的注释信息。
type TableComment struct {
	Comment string
	Columns map[string]string
}

var commonColumnComments = map[string]string{
	"id":         "主键ID",
	"created_at": "创建时间",
	"updated_at": "更新时间",
	"deleted_at": "软删除时间",
}

var schemaComments = map[string]TableComment{
	"role": table("角色表", columns{
		"id":   "主键ID",
		"name": "角色名称",
	}),
	"user": table("用户账号表", withBase(columns{
		"username":          "登录账号",
		"password":          "密码哈希",
		"password_set":      "是否已设置登录密码",
		"nickname":          "用户昵称",
		"email":             "绑定邮箱",
		"email_verified_at": "主邮箱验证时间",
		"phone":             "绑定手机号",
		"site":              "个人站点地址",
		"avatar_url":        "头像URL",
		"mark":              "身份标签",
		"status":            "账号状态",
		"last_login_at":     "最后登录时间",
		"last_active_at":    "最后活跃时间",
	})),
	"user_role": table("用户角色关联表", columns{
		"id":      "主键ID",
		"user_id": "用户ID",
		"role_id": "角色ID",
	}),
	"user_like": table("用户点赞记录表", withBase(columns{
		"user_id":   "用户ID",
		"target_id": "被点赞目标ID",
		"type":      "被点赞目标类型",
	})),
	"user_meta": table("用户扩展资料表", columns{
		"user_id":               "用户ID",
		"name":                  "真实姓名",
		"description":           "个人简介",
		"sub_email":             "副邮箱",
		"sub_email_verified_at": "副邮箱验证时间",
		"gender":                "性别",
		"birthday":              "生日",
		"id_card":               "身份证号",
		"country":               "国家",
		"province":              "省份",
		"city":                  "城市",
		"address":               "详细地址",
		"created_at":            "创建时间",
		"updated_at":            "更新时间",
	}),
	"user_setting": table("用户偏好设置表", columns{
		"user_id":       "用户ID",
		"mail_show":     "对外显示邮箱来源",
		"mail_receive":  "接收邮件邮箱来源",
		"dark_mode":     "暗黑模式偏好",
		"receive_mail":  "是否接收邮件",
		"show_name":     "是否展示真实姓名",
		"show_age":      "是否展示年龄",
		"show_phone":    "是否展示手机号",
		"show_qq":       "是否展示QQ",
		"show_wechat":   "是否展示微信",
		"show_zhihu":    "是否展示知乎",
		"show_sina":     "是否展示微博",
		"show_bili":     "是否展示B站",
		"show_position": "是否展示所在位置",
		"created_at":    "创建时间",
		"updated_at":    "更新时间",
	}),
	"user_social_link": table("用户社交链接表", withBase(columns{
		"user_id":  "用户ID",
		"platform": "社交平台标识",
		"url":      "社交账号链接或账号值",
	})),
	"social_user": table("第三方用户身份表", withBase(columns{
		"uuid":          "第三方系统唯一ID",
		"source":        "第三方来源平台",
		"access_token":  "第三方授权令牌",
		"refresh_token": "第三方刷新令牌",
		"open_id":       "第三方OpenID",
		"is_active":     "第三方身份是否有效",
	})),
	"social_user_auth": table("用户第三方身份绑定表", withBase(columns{
		"user_id":        "系统用户ID",
		"social_user_id": "第三方用户身份ID",
	})),
	"article": table("文章表", withBase(columns{
		"title":                 "文章标题",
		"cover_img_url":         "封面图URL",
		"mobile_cover_img_url":  "移动端封面图URL",
		"short_content":         "文章摘要",
		"content":               "文章正文",
		"user_id":               "作者用户ID",
		"status":                "文章状态",
		"comment_status":        "评论开关状态",
		"password":              "阅读密码",
		"read_count":            "阅读次数",
		"cover_ai_generated":    "封面是否AI生成",
		"content_ai_referenced": "正文是否参考AI",
	})),
	"article_recommend": table("文章推荐表", withBase(columns{
		"article_id": "文章ID",
		"seq":        "推荐排序",
	})),
	"article_category": table("文章分类关联表", columns{
		"id":          "主键ID",
		"article_id":  "文章ID",
		"category_id": "分类ID",
	}),
	"article_tag": table("文章标签关联表", columns{
		"id":         "主键ID",
		"article_id": "文章ID",
		"tag_id":     "标签ID",
		"seq":        "展示排序",
	}),
	"article_music": table("文章音乐关联表", columns{
		"id":         "主键ID",
		"article_id": "文章ID",
		"music_id":   "音乐ID",
	}),
	"article_ai_model": table("文章AI模型披露表", columns{
		"id":         "主键ID",
		"article_id": "文章ID",
		"scope":      "AI使用场景",
		"model_name": "AI模型名称",
		"seq":        "展示排序",
	}),
	"category": table("分类表", withBase(columns{
		"parent_id":     "父分类ID",
		"name":          "分类名称",
		"url":           "分类路由别名",
		"icon":          "分类图标URL",
		"description":   "分类描述",
		"cover_img_url": "分类封面图URL",
		"seq":           "展示排序",
	})),
	"tag": table("标签表", withBase(columns{
		"name":          "标签名称",
		"url":           "标签路由别名",
		"icon":          "标签图标URL",
		"description":   "标签描述",
		"cover_img_url": "标签封面图URL",
		"seq":           "展示排序",
	})),
	"music_artist": table("音乐艺术家表", withBase(columns{
		"name":        "艺术家名称",
		"name_zh":     "艺术家中文名称",
		"avatar_key":  "艺术家头像对象键",
		"description": "艺术家描述",
	})),
	"music_album": table("音乐专辑表", withBase(columns{
		"name":         "专辑名称",
		"artist_id":    "专辑艺术家ID",
		"cover_key":    "专辑封面对象键",
		"release_date": "发行日期",
		"description":  "专辑描述",
	})),
	"music_artist_relation": table("音乐艺术家关系表", columns{
		"id":        "主键ID",
		"music_id":  "音乐ID",
		"artist_id": "艺术家ID",
		"role":      "艺术家角色",
		"seq":       "展示排序",
	}),
	"music": table("音乐表", withBase(columns{
		"name":                "歌曲名称",
		"singer":              "歌手文本",
		"artist_display_name": "展示艺术家名称",
		"album":               "专辑文本",
		"album_id":            "专辑ID",
		"album_track_no":      "专辑曲目编号",
		"song_date":           "歌曲日期",
		"audio_key":           "音频对象键",
		"audio_size":          "音频文件大小",
		"audio_mime":          "音频MIME类型",
		"audio_hash":          "音频哈希",
		"cover_img_url":       "封面图URL",
		"description":         "歌曲描述",
		"lyric":               "歌词",
		"duration":            "时长秒数",
		"seq":                 "展示排序",
		"is_public":           "是否公开展示",
	})),
	"moment": table("碎语表", withBase(columns{
		"user_id":        "作者用户ID",
		"content":        "碎语正文",
		"status":         "公开状态",
		"comment_status": "评论开关状态",
		"read_count":     "阅读次数",
		"is_top":         "是否置顶",
	})),
	"friend_link": table("友链表", withBase(columns{
		"name":        "网站名称",
		"description": "网站描述",
		"email":       "站长邮箱",
		"phone":       "联系电话",
		"site":        "网站URL",
		"avatar_url":  "网站头像或Logo",
		"seq":         "展示排序",
		"status":      "友链状态",
	})),
	"moment_media": table("碎语媒体表", withBase(columns{
		"uploader_id": "上传者用户ID",
		"moment_id":   "碎语ID",
		"type":        "媒体类型",
		"file_type":   "文件扩展名",
		"name":        "原始文件名",
		"url":         "访问URL",
		"size":        "文件大小字节数",
		"status":      "媒体状态",
		"seq":         "展示排序",
		"read_count":  "查看次数",
	})),
	"article_comment": table("文章评论表", withBase(columns{
		"article_id": "文章ID",
		"user_id":    "评论者用户ID",
		"content":    "评论内容",
	})),
	"moment_comment": table("碎语评论表", withBase(columns{
		"moment_id": "碎语ID",
		"user_id":   "评论者用户ID",
		"content":   "评论内容",
	})),
	"guestbook": table("留言表", withBase(columns{
		"owner_user_id": "被留言用户ID",
		"from_user_id":  "留言者用户ID",
		"content":       "留言内容",
	})),
	"article_comment_reply": table("文章评论回复表", withBase(replyColumns("所属文章评论ID"))),
	"moment_comment_reply":  table("碎语评论回复表", withBase(replyColumns("所属碎语评论ID"))),
	"guestbook_reply":       table("留言回复表", withBase(replyColumns("所属留言ID"))),
	"notification_event": table("通知事件表", withBase(columns{
		"type":            "通知事件类型",
		"actor_user_id":   "操作人用户ID",
		"source_type":     "直接对象类型",
		"source_id":       "直接对象ID",
		"root_type":       "根对象类型",
		"root_id":         "根对象ID",
		"title":           "事件标题快照",
		"content_excerpt": "内容摘要快照",
		"metadata_json":   "事件扩展信息JSON",
		"dispatch_status": "分发状态",
		"attempts":        "分发尝试次数",
		"next_process_at": "下次可处理时间",
		"lease_until":     "worker租约到期时间",
		"locked_by":       "领取事件的worker标识",
		"last_error":      "最近一次分发错误",
	})),
	"notification_inbox": table("站内通知收件箱表", withBase(columns{
		"event_id":          "通知事件ID",
		"recipient_user_id": "接收人用户ID",
		"is_read":           "是否已读",
		"read_at":           "已读时间",
		"delivered_at":      "投递时间",
	})),
	"notification_preference": table("通知偏好表", columns{
		"user_id":           "用户ID",
		"event_type":        "通知事件类型",
		"in_app_enabled":    "是否接收站内通知",
		"email_enabled":     "是否接收邮件通知",
		"email_digest_mode": "邮件摘要模式",
		"quiet_start":       "静默时段开始时间",
		"quiet_end":         "静默时段结束时间",
	}),
	"notification_email_task": table("通知邮件任务表", withBase(columns{
		"event_id":          "通知事件ID",
		"recipient_user_id": "接收人用户ID",
		"actor_user_id":     "操作人用户ID",
		"to_email":          "发送目标邮箱快照",
		"event_type":        "事件类型快照",
		"purpose":           "邮件用途",
		"priority":          "处理优先级",
		"status":            "任务状态",
		"available_at":      "最早可聚合时间",
		"next_attempt_at":   "下次处理时间",
		"attempts":          "尝试次数",
		"batch_id":          "归属邮件批次ID",
		"lease_until":       "worker租约到期时间",
		"locked_by":         "领取任务的worker标识",
		"idempotency_key":   "任务幂等键",
		"last_error":        "最近一次处理错误",
	})),
	"notification_email_batch": table("通知邮件批次表", withBase(columns{
		"recipient_user_id": "接收人用户ID",
		"to_email":          "收件邮箱快照",
		"purpose":           "邮件用途",
		"subject":           "邮件标题",
		"status":            "批次状态",
		"item_count":        "包含任务数",
		"scheduled_at":      "计划发送时间",
		"sent_at":           "实际发送时间",
		"attempts":          "尝试次数",
		"lease_until":       "worker租约到期时间",
		"locked_by":         "领取批次的worker标识",
		"message_id":        "邮件Message-ID或内部幂等ID",
		"last_error":        "最近一次发送错误",
	})),
	"notification_email_batch_item": table("通知邮件批次任务关联表", columns{
		"id":       "主键ID",
		"batch_id": "邮件批次ID",
		"task_id":  "邮件任务ID",
	}),
	"email_quota_policy": table("邮件用途额度策略表", columns{
		"purpose":        "邮件用途",
		"daily_limit":    "每日上限",
		"reserved_min":   "每日保底份额",
		"priority":       "全局优先级",
		"max_per_minute": "每分钟上限",
		"max_per_hour":   "每小时上限",
		"enabled":        "是否启用",
	}),
	"email_role_quota_policy": table("邮件角色额度策略表", columns{
		"role":         "角色标识",
		"scope_type":   "限额维度",
		"daily_limit":  "每日上限",
		"max_per_hour": "每小时上限",
		"enabled":      "是否启用",
	}),
	"email_quota_usage": table("邮件额度使用量表", columns{
		"quota_date":   "统计日期",
		"scope_type":   "限额维度",
		"scope_id":     "限额对象ID",
		"purpose":      "邮件用途",
		"window_type":  "统计窗口类型",
		"window_start": "统计窗口开始时间",
		"used_count":   "已用数量",
	}),
	"email_send_log": table("邮件发送日志表", columns{
		"id":         "主键ID",
		"batch_id":   "通知邮件批次ID",
		"purpose":    "邮件用途",
		"to_email":   "收件邮箱",
		"status":     "发送结果",
		"provider":   "邮件供应商",
		"message_id": "邮件Message-ID",
		"error":      "发送错误",
		"created_at": "创建时间",
	}),
	"analytics_events": table("访问分析原始事件表", columns{
		"id":               "主键ID",
		"event_type":       "事件类型",
		"visitor_id":       "访客ID",
		"user_id":          "登录用户ID",
		"is_authenticated": "是否已登录",
		"session_id":       "会话ID",
		"path":             "访问路径",
		"title":            "页面标题",
		"referer_host":     "来源域名",
		"referer_type":     "来源类型",
		"device_type":      "设备类型",
		"browser":          "浏览器",
		"os":               "操作系统",
		"country":          "国家",
		"region":           "地区",
		"city":             "城市",
		"isp":              "网络运营商",
		"country_code":     "国家代码",
		"ip_hash":          "IP哈希",
		"is_new_visitor":   "是否新访客",
		"is_bot":           "是否机器人访问",
		"bot_reason":       "机器人判定原因",
		"is_suspect":       "是否可疑访问",
		"suspect_reason":   "可疑访问原因",
		"created_at":       "创建时间",
	}),
	"analytics_sessions": table("访问分析会话表", columns{
		"session_id":       "会话ID",
		"visitor_id":       "访客ID",
		"user_id":          "登录用户ID",
		"is_authenticated": "是否已登录",
		"first_seen":       "首次访问时间",
		"last_seen":        "最后访问时间",
		"pv_count":         "页面浏览次数",
		"entry_path":       "入口路径",
		"exit_path":        "退出路径",
		"duration":         "会话时长秒数",
		"is_bounce":        "是否跳出会话",
		"device_type":      "设备类型",
		"browser":          "浏览器",
		"os":               "操作系统",
		"country":          "国家",
		"region":           "地区",
		"city":             "城市",
		"isp":              "网络运营商",
		"country_code":     "国家代码",
		"referer_type":     "来源类型",
		"is_bot":           "是否机器人访问",
		"is_suspect":       "是否可疑访问",
	}),
	"analytics_daily": table("每日站点访问汇总表", columns{
		"date":          "统计日期",
		"pv":            "页面浏览量",
		"uv":            "独立访客数",
		"sessions":      "会话数",
		"new_visitors":  "新访客数",
		"avg_duration":  "平均会话时长秒数",
		"bounce_rate":   "跳出率",
		"registered_pv": "注册用户页面浏览量",
		"registered_uv": "注册用户独立访客数",
		"anonymous_pv":  "匿名用户页面浏览量",
		"anonymous_uv":  "匿名用户独立访客数",
	}),
	"analytics_daily_dim": table("每日访问维度汇总表", columns{
		"date":      "统计日期",
		"dimension": "统计维度",
		"dim_value": "维度值",
		"pv":        "页面浏览量",
		"uv":        "独立访客数",
	}),
	"analytics_page_daily": table("每日页面访问汇总表", columns{
		"date":  "统计日期",
		"path":  "访问路径",
		"title": "页面标题",
		"pv":    "页面浏览量",
		"uv":    "独立访客数",
	}),
	"analytics_friend_link_daily": table("每日友链来源访问汇总表", columns{
		"date":           "统计日期",
		"friend_link_id": "友链ID",
		"friend_name":    "友链名称",
		"site":           "友链URL",
		"site_host":      "友链域名",
		"pv":             "页面浏览量",
		"uv":             "独立访客数",
		"sessions":       "会话数",
	}),
	"moderation_item": table("内容审核对象表", columns{
		"id":                       "主键ID",
		"content_type":             "业务内容类型",
		"content_id":               "业务内容ID",
		"author_id":                "作者用户ID",
		"lifecycle_state":          "生命周期状态",
		"public_state":             "公开展示状态",
		"materialized_revision_id": "当前物化修订ID",
		"approved_revision_id":     "已通过修订ID",
		"pending_revision_id":      "待审核修订ID",
		"state_before_emergency":   "紧急隐藏前公开状态",
		"emergency_hidden_reason":  "紧急隐藏原因",
		"emergency_hidden_at":      "紧急隐藏时间",
		"deleted_at":               "软删除时间",
		"lock_version":             "乐观锁版本",
		"created_at":               "创建时间",
		"updated_at":               "更新时间",
	}),
	"moderation_revision": table("内容审核修订表", columns{
		"id":                     "主键ID",
		"item_id":                "审核对象ID",
		"version":                "修订版本号",
		"submitter_id":           "提交者用户ID",
		"idempotency_key":        "提交幂等键",
		"submitted_content":      "提交原文",
		"published_content":      "发布正文",
		"risk_level":             "风险等级",
		"policy_action":          "策略动作",
		"review_status":          "审核状态",
		"moment_status":          "碎语提交时公开开关",
		"moment_comment_status":  "碎语提交时评论开关",
		"ruleset_version":        "规则集版本",
		"rule_match_ids":         "命中规则ID列表",
		"rule_matches_truncated": "命中规则列表是否被截断",
		"decision_type":          "审核决定类型",
		"decision_reason":        "审核决定原因",
		"reviewer_id":            "审核人用户ID",
		"reviewed_at":            "审核时间",
		"created_at":             "创建时间",
		"updated_at":             "更新时间",
	}),
	"moderation_revision_image": table("审核修订图片表", columns{
		"id":          "主键ID",
		"revision_id": "审核修订ID",
		"seq":         "图片排序",
		"object_key":  "图片对象键",
		"sha256":      "图片SHA256",
		"md5":         "图片MD5",
		"size":        "图片大小字节数",
		"media_type":  "图片媒体类型",
		"is_gif":      "是否GIF图片",
		"created_at":  "创建时间",
		"updated_at":  "更新时间",
	}),
	"moderation_image": table("审核图片指纹表", columns{
		"id":                 "主键ID",
		"sha256":             "图片SHA256",
		"size":               "图片大小字节数",
		"md5":                "图片MD5",
		"status":             "图片审核状态",
		"preview_object_key": "预览图对象键",
		"approved_at":        "通过时间",
		"approved_by":        "通过审核人用户ID",
		"last_used_at":       "最后使用时间",
		"created_at":         "创建时间",
		"updated_at":         "更新时间",
	}),
	"moderation_attempt": table("内容审核尝试表", columns{
		"id":                     "主键ID",
		"user_id":                "提交用户ID",
		"content_type":           "业务内容类型",
		"item_id":                "审核对象ID",
		"idempotency_key":        "提交幂等键",
		"ruleset_version":        "规则集版本",
		"rule_match_ids":         "命中规则ID列表",
		"rule_matches_truncated": "命中规则列表是否被截断",
		"created_at":             "创建时间",
	}),
	"moderation_rule_source": table("审核规则来源表", columns{
		"id":         "主键ID",
		"name":       "规则来源名称",
		"created_at": "创建时间",
		"updated_at": "更新时间",
	}),
	"moderation_ruleset": table("审核规则集表", columns{
		"id":                   "主键ID",
		"base_ruleset_id":      "基础规则集ID",
		"status":               "规则集状态",
		"rule_count":           "规则总数",
		"keyword_count":        "关键词规则数",
		"regexp_count":         "正则规则数",
		"composite_count":      "组合规则数",
		"index_bytes":          "索引字节数",
		"build_peak_bytes":     "构建峰值内存字节数",
		"build_duration_ms":    "构建耗时毫秒数",
		"index_object_key":     "索引对象键",
		"index_format_version": "索引格式版本",
		"index_sha256":         "索引SHA256",
		"operator_id":          "操作人用户ID",
		"failure_code":         "失败代码",
		"created_at":           "创建时间",
		"updated_at":           "更新时间",
	}),
	"moderation_rule": table("审核规则表", columns{
		"id":                     "主键ID",
		"name":                   "规则名称",
		"rule_type":              "规则类型",
		"pattern":                "规则匹配模式",
		"dedupe_hash":            "去重哈希",
		"category":               "规则分类",
		"effect":                 "规则效果",
		"risk_level":             "风险等级",
		"priority":               "匹配优先级",
		"source_id":              "规则来源ID",
		"activated_ruleset_id":   "启用规则集ID",
		"deactivated_ruleset_id": "停用规则集ID",
		"replaces_rule_id":       "替代规则ID",
		"created_at":             "创建时间",
		"updated_at":             "更新时间",
	}),
	"moderation_ruleset_removal": table("审核规则集移除记录表", columns{
		"ruleset_id": "规则集ID",
		"rule_id":    "规则ID",
		"created_at": "创建时间",
	}),
	"moderation_rule_import": table("审核规则导入任务表", columns{
		"id":                 "主键ID",
		"file_name":          "导入文件名",
		"format":             "导入文件格式",
		"file_size":          "导入文件大小",
		"object_key":         "导入文件对象键",
		"source_id":          "规则来源ID",
		"default_category":   "默认规则分类",
		"default_effect":     "默认规则效果",
		"default_risk_level": "默认风险等级",
		"default_priority":   "默认优先级",
		"validation_status":  "校验状态",
		"total_rows":         "总行数",
		"valid_rows":         "有效行数",
		"duplicate_rows":     "重复行数",
		"error_rows":         "错误行数",
		"error_object_key":   "错误明细对象键",
		"ruleset_id":         "生成规则集ID",
		"operator_id":        "操作人用户ID",
		"created_at":         "创建时间",
		"updated_at":         "更新时间",
	}),
	"moderation_action_log": table("审核操作日志表", columns{
		"id":              "主键ID",
		"item_id":         "审核对象ID",
		"revision_id":     "审核修订ID",
		"actor_user_id":   "操作人用户ID",
		"subject_user_id": "受影响用户ID",
		"action":          "操作类型",
		"reason":          "操作原因",
		"metadata_json":   "操作扩展信息JSON",
		"created_at":      "创建时间",
	}),
	"moderation_visible_image": table("审核可见图片表", columns{
		"id":          "主键ID",
		"item_id":     "审核对象ID",
		"revision_id": "审核修订ID",
		"seq":         "图片排序",
		"object_key":  "图片对象键",
		"created_at":  "创建时间",
		"updated_at":  "更新时间",
	}),
	"user_moderation_profile": table("用户审核画像表", columns{
		"user_id":               "用户ID",
		"trust_level":           "信任等级",
		"trust_source":          "信任来源",
		"manual_trust_locked":   "是否手动锁定信任",
		"sanction_state":        "处置状态",
		"sanction_until":        "处置截止时间",
		"sanction_reason":       "处置原因",
		"clean_approval_streak": "连续干净通过次数",
		"corrected_count":       "被修正次数",
		"rejected_count":        "被拒绝次数",
		"high_risk_count":       "高风险次数",
		"violation_score":       "违规分数",
		"last_violation_at":     "最后违规时间",
		"restricted_until":      "限制截止时间",
		"created_at":            "创建时间",
		"updated_at":            "更新时间",
	}),
	"moderation_control": table("审核全局控制表", columns{
		"id":                "主键ID",
		"registration_mode": "注册控制模式",
		"publishing_mode":   "发布控制模式",
		"reason":            "控制原因",
		"operator_id":       "操作人用户ID",
		"changed_at":        "变更时间",
		"lock_version":      "乐观锁版本",
		"created_at":        "创建时间",
		"updated_at":        "更新时间",
	}),
	"moderation_review_email_batch": table("审核摘要邮件批次表", columns{
		"id":                "主键ID",
		"recipient_user_id": "接收人用户ID",
		"to_email":          "收件邮箱快照",
		"subject":           "邮件标题",
		"status":            "批次状态",
		"item_count":        "包含任务数",
		"scheduled_at":      "计划发送时间",
		"sent_at":           "实际发送时间",
		"attempts":          "尝试次数",
		"next_attempt_at":   "下次尝试时间",
		"lease_until":       "worker租约到期时间",
		"locked_by":         "领取批次的worker标识",
		"message_id":        "邮件Message-ID",
		"last_error":        "最近一次发送错误",
		"created_at":        "创建时间",
		"updated_at":        "更新时间",
	}),
	"moderation_review_email_task": table("审核摘要邮件任务表", columns{
		"id":              "主键ID",
		"revision_id":     "待审核内容修订ID",
		"item_id":         "待审核内容对象ID",
		"status":          "任务状态",
		"available_at":    "最早可处理时间",
		"next_attempt_at": "下次尝试时间",
		"batch_id":        "归属审核摘要邮件批次ID",
		"created_at":      "创建时间",
		"updated_at":      "更新时间",
	}),
}

// SchemaComments 返回当前数据库注释 catalog。
func SchemaComments() SchemaCommentSet {
	return SchemaCommentSet{Tables: schemaComments}
}

// BuildSchemaCommentSQL 根据当前注释 catalog 与现有列定义生成注释 SQL。
func BuildSchemaCommentSQL(tableDefinitions map[string]map[string]string) ([]string, error) {
	return buildSchemaCommentSQL(SchemaComments(), tableDefinitions)
}

// ApplySchemaComments 在 AutoMigrate 后补齐表与列的中文注释。
func ApplySchemaComments(db *gorm.DB) error {
	definitions, err := loadColumnDefinitions(db)
	if err != nil {
		return err
	}

	statements, err := BuildSchemaCommentSQL(definitions)
	if err != nil {
		return err
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	return nil
}

// RegisteredTableNames 返回迁移注册模型对应的表名。
func RegisteredTableNames() []string {
	models := append([]any{}, coreModels()...)
	models = append(models, moderationModels()...)

	names := make([]string, 0, len(models))
	for _, m := range models {
		if named, ok := m.(interface{ TableName() string }); ok {
			names = append(names, named.TableName())
		}
	}
	return names
}

func quoteSQLComment(comment string) string {
	escaped := strings.ReplaceAll(comment, "'", "''")
	return "'" + escaped + "'"
}

const columnDefinitionsQuery = `
SELECT
	TABLE_NAME,
	COLUMN_NAME,
	COLUMN_TYPE,
	IS_NULLABLE,
	COLUMN_DEFAULT,
	EXTRA,
	GENERATION_EXPRESSION,
	CHARACTER_SET_NAME,
	COLLATION_NAME
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
ORDER BY TABLE_NAME, ORDINAL_POSITION`

type columnDefinitionRow struct {
	TableName            string         `gorm:"column:TABLE_NAME"`
	ColumnName           string         `gorm:"column:COLUMN_NAME"`
	ColumnType           string         `gorm:"column:COLUMN_TYPE"`
	IsNullable           string         `gorm:"column:IS_NULLABLE"`
	ColumnDefault        sql.NullString `gorm:"column:COLUMN_DEFAULT"`
	Extra                string         `gorm:"column:EXTRA"`
	GenerationExpression sql.NullString `gorm:"column:GENERATION_EXPRESSION"`
	CharacterSetName     sql.NullString `gorm:"column:CHARACTER_SET_NAME"`
	CollationName        sql.NullString `gorm:"column:COLLATION_NAME"`
}

func buildSchemaCommentSQL(comments SchemaCommentSet, tableDefinitions map[string]map[string]string) ([]string, error) {
	statements := make([]string, 0)

	for _, table := range sortedTableNames(comments.Tables) {
		tc := comments.Tables[table]
		statements = append(statements, "ALTER TABLE "+quoteIdentifier(table)+" COMMENT = "+quoteSQLComment(tc.Comment))
		for _, column := range sortedColumnNames(tc.Columns) {
			definition, ok := tableDefinitions[table][column]
			if !ok {
				return nil, fmt.Errorf("missing column definition for %s.%s", table, column)
			}
			statements = append(statements, "ALTER TABLE "+quoteIdentifier(table)+" MODIFY COLUMN "+definition+" COMMENT "+quoteSQLComment(tc.Columns[column]))
		}
	}

	return statements, nil
}

func loadColumnDefinitions(db *gorm.DB) (map[string]map[string]string, error) {
	var rows []columnDefinitionRow
	if err := db.Raw(columnDefinitionsQuery).Scan(&rows).Error; err != nil {
		return nil, err
	}

	definitions := make(map[string]map[string]string)
	for _, row := range rows {
		if definitions[row.TableName] == nil {
			definitions[row.TableName] = make(map[string]string)
		}
		definitions[row.TableName][row.ColumnName] = buildColumnDefinition(row)
	}

	return definitions, nil
}

func buildColumnDefinition(row columnDefinitionRow) string {
	parts := []string{quoteIdentifier(row.ColumnName), row.ColumnType}
	parts = appendCharacterDefinition(parts, row)

	if row.GenerationExpression.Valid && row.GenerationExpression.String != "" {
		parts = append(parts, "GENERATED ALWAYS AS ("+row.GenerationExpression.String+")")
		parts = append(parts, generatedStorage(row.Extra))
		return strings.Join(parts, " ")
	}

	parts = append(parts, nullableDefinition(row.IsNullable))
	parts = appendDefaultDefinition(parts, row)
	parts = appendExtraDefinition(parts, row.Extra)

	return strings.Join(parts, " ")
}

func appendCharacterDefinition(parts []string, row columnDefinitionRow) []string {
	if row.CharacterSetName.Valid && row.CharacterSetName.String != "" {
		parts = append(parts, "CHARACTER SET "+quoteIdentifierPart(row.CharacterSetName.String))
	}
	if row.CollationName.Valid && row.CollationName.String != "" {
		parts = append(parts, "COLLATE "+quoteIdentifierPart(row.CollationName.String))
	}
	return parts
}

func appendDefaultDefinition(parts []string, row columnDefinitionRow) []string {
	if row.ColumnDefault.Valid {
		return append(parts, "DEFAULT "+formatDefaultValue(row.ColumnDefault.String, row))
	}
	if strings.EqualFold(row.IsNullable, "YES") {
		return append(parts, "DEFAULT NULL")
	}
	return parts
}

func appendExtraDefinition(parts []string, extra string) []string {
	for _, token := range normalizedExtraTokens(extra) {
		parts = append(parts, token)
	}
	return parts
}

func formatDefaultValue(value string, row columnDefinitionRow) string {
	if value == "NULL" {
		return "NULL"
	}
	if isGeneratedDefault(row.Extra) || isUnquotedDefault(value, row.ColumnType) {
		return value
	}
	return quoteSQLComment(value)
}

func isGeneratedDefault(extra string) bool {
	return strings.Contains(strings.ToLower(extra), "default_generated")
}

func isUnquotedDefault(value string, columnType string) bool {
	upperValue := strings.ToUpper(value)
	baseType := strings.ToLower(columnType)
	if index := strings.Index(baseType, "("); index >= 0 {
		baseType = baseType[:index]
	}
	if isTemporalType(baseType) && (upperValue == "CURRENT_TIMESTAMP" || strings.HasPrefix(upperValue, "CURRENT_TIMESTAMP(")) {
		return true
	}
	switch baseType {
	case "bit", "bool", "boolean", "tinyint", "smallint", "mediumint", "int", "integer", "bigint",
		"decimal", "dec", "numeric", "float", "double", "real", "year":
		return true
	default:
		return false
	}
}

func isTemporalType(baseType string) bool {
	switch baseType {
	case "timestamp", "datetime", "date", "time":
		return true
	default:
		return false
	}
}

func normalizedExtraTokens(extra string) []string {
	lower := strings.ToLower(extra)
	tokens := make([]string, 0, 2)
	if strings.Contains(lower, "auto_increment") {
		tokens = append(tokens, "AUTO_INCREMENT")
	}
	if strings.Contains(lower, "on update current_timestamp") {
		start := strings.Index(lower, "on update")
		tokens = append(tokens, "ON UPDATE "+extra[start+len("on update "):])
	}
	if strings.Contains(lower, "invisible") {
		tokens = append(tokens, "INVISIBLE")
	}
	return tokens
}

func generatedStorage(extra string) string {
	lower := strings.ToLower(extra)
	if strings.Contains(lower, "stored generated") {
		return "STORED"
	}
	return "VIRTUAL"
}

func nullableDefinition(isNullable string) string {
	if strings.EqualFold(isNullable, "YES") {
		return "NULL"
	}
	return "NOT NULL"
}

func sortedTableNames(tables map[string]TableComment) []string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedColumnNames(columns map[string]string) []string {
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func quoteIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func quoteIdentifierPart(identifier string) string {
	return strings.ReplaceAll(identifier, "`", "``")
}

type columns map[string]string

func table(comment string, cols columns) TableComment {
	return TableComment{Comment: comment, Columns: cols}
}

func withBase(cols columns) columns {
	result := make(columns, len(commonColumnComments)+len(cols))
	for name, comment := range commonColumnComments {
		result[name] = comment
	}
	for name, comment := range cols {
		result[name] = comment
	}
	return result
}

func replyColumns(commentIDComment string) columns {
	return columns{
		"comment_id":      commentIDComment,
		"to_user_id":      "被回复者用户ID",
		"from_user_id":    "回复者用户ID",
		"parent_reply_id": "上级回复ID",
		"content":         "回复内容",
	}
}
