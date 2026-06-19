# migrate-garage-articles

迁移 article 表中的历史 Garage 图片路径：

- `cover_img_url` 迁移到 `articles/{article_id}/cover/{filename}`。
- `content` 中的 `post/images/...` 迁移到 `articles/{article_id}/images/{filename}`。

脚本默认 dry-run，不复制对象、不更新数据库：

```bash
go run ./cmd/migrate-garage-articles --dry-run
```

确认计划无误后执行真实迁移：

```bash
go run ./cmd/migrate-garage-articles --apply
```

执行依赖 `config/config.local.yaml` 中的 `db` 和 `garage` 配置。迁移会保留旧对象；如果目标对象已存在，会跳过复制并继续更新数据库。

结束时会输出统计和失败明细。失败明细包含 `article_id`、`stage`、`source`、`target` 和 `error`，方便定位具体失败对象。
