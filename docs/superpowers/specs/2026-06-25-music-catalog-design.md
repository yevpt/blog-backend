# 音乐资料库设计

## 背景

当前音乐模块只有公开列表接口，`music` 表直接保存曲名、歌手、专辑、音频 URL、封面 URL 等展示字段。它可以支撑文章背景音乐列表，但不足以支撑后续歌手头像、专辑封面复用、合唱歌曲、歌单分享和 Garage 音乐文件上传。

本次设计目标是把音乐模块升级为个人博客用的轻量音乐资料库。音乐均由站长上传，不需要审核流，不引入复杂音乐平台模型。

## 目标

- 支持歌手、专辑、歌曲独立管理。
- 支持合唱、feat 等多歌手歌曲。
- 支持歌手中文译名，并由后端返回统一展示名。
- 支持歌手头像、专辑封面、歌曲音频上传到 Garage。
- 迁移旧 `music` 数据和旧 Garage 路径，保持现有文章背景音乐可用。
- 为后续播放列表、歌单分享预留清晰数据边界。

## 非目标

- 不做音乐审核。
- 不做版权、发行商、曲作者、作词作曲等完整音乐平台模型。
- 不做歌手多语言别名表；仅保留 `name_zh` 作为中文译名。
- 不在第一阶段做音频转码、波形图、歌词时间轴编辑。

## 数据模型

### music_artist

歌手实体。

```text
id
name             原名或标准名，例如 문성남
name_zh          中文译名，可空，例如 文胜南
avatar_key       歌手头像 Garage key，可空
description      简介，可空
created_at
updated_at
deleted_at
```

后端返回：

```text
display_name = name_zh 非空 ? name + " (" + name_zh + ")" : name
```

搜索歌手时同时匹配 `name` 和 `name_zh`。

### music_album

专辑实体。封面归专辑，不归单曲，多首同专辑歌曲共用一个封面。

```text
id
name             专辑名
artist_id        专辑主歌手，可空
cover_key        专辑封面 Garage key，可空
release_date     专辑发布时间，可空
description      简介，可空
created_at
updated_at
deleted_at
```

建议建立 `name + artist_id` 唯一约束，避免同一歌手下重复专辑。

### music

歌曲实体。

```text
id
name
album_id              可空
artist_display_name   歌曲歌手展示名，保留最终展示字符串
album_track_no        专辑序号，可空或 0
audio_key             音频 Garage key
lyric                 歌词，可空
duration              秒
audio_size            字节数
audio_mime            真实 MIME
audio_hash            内容 hash，用于去重
is_public             是否出现在公开音乐库
seq                   全局排序
created_at
updated_at
deleted_at
```

`artist_display_name` 用于稳定展示和兼容旧数据。结构化筛选仍以 `music_artist_relation` 为准。

### music_artist_relation

歌曲与歌手的多对多关系，支持合唱和 feat。

```text
id
music_id
artist_id
role        primary / featured，第一阶段可默认 primary
seq         展示顺序
```

建议建立 `music_id + artist_id + role` 唯一约束。

## Garage 路径

正式路径：

```text
music/audio/{music_id}/{hash}.{ext}
music/albums/{album_id}/cover/{hash}.{ext}
music/artists/{artist_id}/avatar/{hash}.{ext}
```

临时上传路径：

```text
temp/music/{user_id}/audio/{hash}.{ext}
temp/music/{user_id}/album-cover/{hash}.{ext}
temp/music/{user_id}/artist-avatar/{hash}.{ext}
```

上传后先进入临时路径；保存实体成功后再复制或移动到正式路径。涉及 DB 和 Garage 的写操作必须做补偿：

- 对象成功、DB 失败：删除本次新对象。
- DB 成功、旧对象不再引用：提交后清理旧对象。
- 迁移旧路径：先 copy 到新路径，不立刻删除旧 key。

## 上传与解析

音频上传接口负责：

- 限制文件大小和类型。
- 校验真实音频内容。
- 计算 hash、size、mime。
- 解析歌曲名、歌手、专辑、专辑序号、发行日期、时长、内嵌封面。
- 返回建议字段，供后台表单确认。

自动解析结果只作为预填建议。最终入库以后台提交为准。

封面和头像上传接口复用图片真实内容校验，并建议统一压缩到固定格式后再计算 hash，减少同图不同压缩导致的重复对象。

## 接口规划

公开接口：

```text
GET /music
GET /music/:id
GET /music/artists
GET /music/artists/:id
GET /music/albums
GET /music/albums/:id
```

管理接口：

```text
GET    /admin/music
POST   /admin/music
PUT    /admin/music/:id
DELETE /admin/music/:id

GET    /admin/music/artists
POST   /admin/music/artists
PUT    /admin/music/artists/:id
DELETE /admin/music/artists/:id

GET    /admin/music/albums
POST   /admin/music/albums
PUT    /admin/music/albums/:id
DELETE /admin/music/albums/:id

POST   /admin/music/uploads/audio
POST   /admin/music/uploads/album-cover
POST   /admin/music/uploads/artist-avatar
```

文章保存接口继续接收 `music_ids`，但需要校验音乐存在。后续歌单分享可以新增 playlist 表，不影响本次核心模型。

## 旧数据迁移

### 数据库迁移

新增 `migrations/20260625_music_catalog.sql`：

1. 新建 `music_artist`、`music_album`、`music_artist_relation`。
2. 为 `music` 增加新字段。
3. 回填歌手：
   - `singer` 匹配 `原名 (中文译名)` 或 `原名（中文译名）` 时，拆成 `name` 和 `name_zh`。
   - 无括号时，`name=singer`，`name_zh=NULL`。
   - 合唱按 `/`、`,`、`、`、`&`、`feat.` 等分隔符尽量拆分。
4. 回填专辑：
   - `album` 写入 `music_album.name`。
   - `song_date` 优先回填到 `music_album.release_date`。
   - `cover_img_url` 回填到 `music_album.cover_key`。
5. 回填歌曲：
   - `url` 回填到 `music.audio_key`。
   - `singer` 原文回填到 `music.artist_display_name`。
   - `album_track_no` 暂填 0，后续后台手动补。
6. 写入 `music_artist_relation`。

旧字段第一阶段保留，不立即删除，便于回滚和对账。

### 旧库重迁工具

同步更新 `cmd/migrate/main.go`：

- `AutoMigrate` 注册新模型。
- `migrateMusic` 直接生成歌手、专辑、歌曲、歌手关系。
- Garage 对象迁移阶段增加音乐音频和专辑封面复制。

### Garage 对象迁移

迁移工具从旧 `url`、`cover_img_url` 解析本站对象 key：

- 可解析为本站 key：copy 到新路径，成功后更新 DB。
- 外部 URL：不搬迁，保留原值并在后台标记为待处理。
- 复制失败：记录失败项，不更新对应 DB 字段。

迁移阶段只 copy，不 move。确认线上稳定后，再单独清理旧 key。

## 分层实现

- `handler` 只负责绑定参数、身份提取、响应选择。
- `service` 负责业务校验、Garage 与 DB 编排、失败补偿、展示字段组装。
- `repository` 只做 GORM 查询和事务保存，返回 `model.*`。
- `dto` 是对外请求和响应的唯一来源，禁止直接返回 `model.*`。

## 风险与对策

- 旧歌手拆分不准：保留 `artist_display_name`，后台可后续修正关系。
- Garage 迁移失败：对象迁移与 DB 更新逐条执行，失败项可重试。
- 外部 URL 混入：只迁移可被 `ObjectKey` 识别的本站对象，外部 URL 不删除不搬迁。
- 音频文件较大：后续实现应考虑流式上传接口，避免一次性读取过大文件进内存。
- 文章关联音乐孤儿数据：迁移和保存时都需要校验 `music_id` 存在。

## 验证

- repository 使用 sqlmock 覆盖查询、保存、回填关系。
- service 使用 fake object store 覆盖上传成功、DB 失败清理、替换旧对象清理。
- handler 使用 httptest 覆盖权限、参数边界、上传大小限制。
- Swagger 更新后运行 `make swag`。
- 至少运行音乐、文章关联、对象存储相关包测试。
