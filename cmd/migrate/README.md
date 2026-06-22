# 数据迁移工具

`cmd/migrate` 用于把旧博客库迁到当前表结构，并同步 Garage 对象路径。请先确认目标库是新库或可重建的迁移库，不要把 `db` 指到旧库或线上正式库。

## 配置

在 `config/config.local.yaml` 中配置目标库、源库和 Garage：

```yaml
db:
  # 目标库：迁移结果会写入这里
  name: blog_dev

migrate:
  # 源库：旧 blog 数据库，只读
  src_dsn: "user:password@tcp(host:3306)/blog?charset=utf8mb4&parseTime=True&loc=Local"

garage:
  bucket: "blog"
  endpoint: "https://garage-s3-api.example.com"
  region: "garage"
  accessKeyID: "your_access_key"
  secretAccessKey: "your_secret_key"
```

也可以临时用环境变量覆盖源库 DSN：

```bash
SRC_DSN='user:password@tcp(host:3306)/blog?charset=utf8mb4&parseTime=True&loc=Local' go run ./cmd/migrate
```

## 推荐流程

第一次建议先只迁数据库，不复制 Garage 对象：

```bash
go run ./cmd/migrate --skip-garage
```

确认目标库数据和表结构正常后，再执行完整迁移：

```bash
go run ./cmd/migrate
```

普通数据迁移步骤有幂等保护：目标表已有数据时会跳过；Garage 迁移会复制对象并更新数据库路径。文章资源会从旧路径 `post/...` 迁到新路径 `articles/{article_id}/...`；碎语媒体会从旧路径 `say/{moment_id}/...` 迁到新路径 `moments/{user_id}/{moment_id}/...`，并同步修正 `moment_media.moment_id`、`moment_media.uploader_id` 和 `moment_media.url`。

## 参数

| 参数 | 说明 |
|------|------|
| `--skip-garage` | 只迁数据库，跳过 Garage 对象复制和路径更新 |
| `--force` | 强制重跑已有数据的迁移步骤，通常只在清空目标库或明确需要重建时使用 |

迁移结束后会清理旧兼容表，包括历史 `media` 表；当前碎语多媒体目标表为 `moment_media`，表内直接使用 `moment_id` 关联碎语。
