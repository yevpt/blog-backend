# 内容审核上线与回滚

本文面向当前个人博客部署。核心原则是：先保持 `moderation.enabled: false` 完成建表、迁移和前端升级，校验通过后才启用审核；不要让旧后端和新后端同时写 UGC。

## 上线顺序

1. 备份生产 MySQL，并确认备份可恢复。

2. 对现有生产库只执行一次版本化迁移：

   ```bash
   mysql < migrations/20260701_moderation_review_email.sql
   ```

   `make dbsetup` 只足够用于新库或开发库初始化；现有生产库必须显式应用上述 migration。后端服务启动不会执行 `AutoMigrate`，不能依赖启动自动补表。

3. 校验审核邮件表、索引、外键和待审数据回填。以下 SQL 用于确认待发送任务、进行中批次和回填缺口；migration 完成后最后一条查询应返回 `0`：

   ```sql
   SELECT COUNT(*) FROM moderation_review_email_task WHERE status = 'pending';
   SELECT COUNT(*) FROM moderation_review_email_batch WHERE status IN ('pending','sending');
   SELECT COUNT(*)
   FROM moderation_revision r
   JOIN moderation_item i ON i.pending_revision_id = r.id
   LEFT JOIN moderation_review_email_task t ON t.revision_id = r.id
   WHERE r.review_status = 'pending' AND t.id IS NULL;
   ```

4. 部署当前后端，但保持生产配置：

   ```yaml
   moderation:
     enabled: false
     mode: enforce
   ```

   此时评论、留言、回复、碎语和临时互动图片仍走原业务路径；审核管理路由和清理 worker 不启动。

5. 检查后端启动日志，确认审核邮件 worker 读取配置并正常启动；如发现表、索引、外键或回填异常，先修复数据库再继续。

6. 在停止旧版 UGC 写入后执行历史迁移。命令按配置的 `moderation.migration.batch_size` 分事务处理，逐批输出下一游标：

   ```bash
   go run ./cmd/moderation-migrate
   ```

   如果因对象缺失等原因中断，修复后使用最后输出的游标续跑：

   ```bash
   go run ./cmd/moderation-migrate --after-type article_comment --after-id 1234
   ```

7. 发布已支持 `Idempotency-Key`、审核状态和图片 `display_mode` 的前端。所有发布和编辑请求都必须生成稳定且非空的幂等键。

8. 执行只读校验，结果必须全部为零：

   ```bash
   go run ./cmd/moderation-migrate --verify-only
   ```

9. 将生产 `moderation.enabled` 改为 `true`、保持 `mode: enforce`，然后只部署当前新版后端。启动失败、规则集为空或配置非法时不要绕过校验。

10. 再次检查后端日志，确认审核邮件 worker 启动且没有 lease、批次或邮件配置错误。

11. 冒烟验证低/中/高风险发布、低/中风险编辑、通过/修正/驳回、图片预览/GIF 占位、删除终止态、紧急隐藏/恢复、禁言和全站发布开关。

## 回滚

若启用后出现影响发布的故障：

1. 将生产 `moderation.enabled` 改回 `false` 并重新部署当前版本。
2. 不回退数据库 schema，不删除审核表、版本、图片指纹或业务表内容。
3. 不运行清理或再次迁移，先保留现场并定位问题。
4. 修复后重新执行 `--verify-only`，再按上线步骤启用。

关闭审核后，待审版本不会继续流转；首次先审后发业务行只保留空正文或隐藏状态，不会把待审正文回退暴露。恢复审核后可继续人工处理。

审核邮件 migration 是加法式变更。紧急回滚时可以直接回滚应用镜像，旧代码会忽略新增的审核邮件表；不要为了回滚删除 `moderation_review_email_task` 或 `moderation_review_email_batch`。恢复并重新部署新版后端后，worker 会基于已持久化的任务、批次和 lease 继续处理。

## 日常维护

- 信任等级、处罚、全站控制和紧急隐藏通过管理接口维护；审核规则作为数据库数据维护并在启动时加载；阈值、批次、保留期和稳定提示在 `config/config.yaml` 维护，改动后重启生效。
- 清理 worker 只在审核开启时运行；它保护当前物化、最后通过、待审和仍有操作日志引用的版本。
- 图片审核记录按 `last_used_at` 和 `approval_retention_days` 清理。每次复用已通过图片都会刷新最后访问时间。
- 生产始终使用 `enforce`；`observe` 仅限本地调试。

## 规则管理上线

审核规则管理 API、批量导入和单实例构建 worker 在审核开启后自动注册路由并启动。初始规则集为空，需通过管理端导入或逐条新增。

### 资源规划

| 规则规模 | 推荐内存 | 构建峰值预估 | 说明 |
|----------|----------|-------------|------|
| ~10 万关键词 | 1 GB | 120–300 MB | 日常维护推荐规模 |
| 接近 50 万关键词 | 2–4 GB | 600 MB–1.2 GB | 设计上限，需基准验证 |

- `max_index_memory_mb` 超过时终止构建，当前快照不变。
- `max_build_peak_memory_mb` 必须大于 `max_index_memory_mb`，默认 1024。
- 索引文件保存在 Garage `moderation/rulesets/` 前缀，失败和 superseded 产物按 `ruleset_artifact_retention_days` 清理。
- 导入原文件和错误报告保存在 `moderation/imports/` 前缀，按 `import_artifact_retention_days` 清理。

### 初始导入

1. 通过管理端下载 CSV 或 TXT 模板，填写规则。
2. 在管理端上传文件，设置来源名称和缺省字段。
3. Worker 自动校验、去重、构建索引；校验失败时可下载错误报告。
4. 构建完成后候选规则集状态为 `ready`；管理员确认后发布。
5. 无导入关联的单条新增/修改/批量启停会自动构建并发布。

### 回滚

规则管理出现故障时：

1. 将 `moderation.enabled` 改回 `false` 并重新部署，规则路由和构建 worker 不启动。
2. 不删除规则表和规则集版本；当前已发布规则集保持不变。
3. 失败候选的未发布规则行由清理 worker 自动删除。

### 基准命令

```bash
go test ./internal/service/moderation/ruleindex -run '^$' -bench 'Benchmark(Build100K|Build500K|Match)' -benchmem -count=1
```

详细基准见 `docs/moderation-rule-index-benchmark.md`。
