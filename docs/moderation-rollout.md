# 内容审核上线与回滚

本文面向当前个人博客部署。核心原则是：先保持 `moderation.enabled: false` 完成建表、迁移和前端升级，校验通过后才启用审核；不要让旧后端和新后端同时写 UGC。

## 上线顺序

1. 部署当前后端，但保持生产配置：

   ```yaml
   moderation:
     enabled: false
     mode: enforce
   ```

   此时评论、留言、回复、碎语和临时互动图片仍走原业务路径；审核管理路由和清理 worker 不启动。

2. 执行 schema 更新并确认审核表、外键和种子存在：

   ```bash
   make dbsetup
   ```

3. 在停止旧版 UGC 写入后执行历史迁移。命令按配置的 `moderation.migration.batch_size` 分事务处理，逐批输出下一游标：

   ```bash
   go run ./cmd/moderation-migrate
   ```

   如果因对象缺失等原因中断，修复后使用最后输出的游标续跑：

   ```bash
   go run ./cmd/moderation-migrate --after-type article_comment --after-id 1234
   ```

4. 发布已支持 `Idempotency-Key`、审核状态和图片 `display_mode` 的前端。所有发布和编辑请求都必须生成稳定且非空的幂等键。

5. 执行只读校验，结果必须全部为零：

   ```bash
   go run ./cmd/moderation-migrate --verify-only
   ```

6. 将生产 `moderation.enabled` 改为 `true`、保持 `mode: enforce`，然后只部署当前新版后端。启动失败、规则集为空或配置非法时不要绕过校验。

7. 冒烟验证低/中/高风险发布、低/中风险编辑、通过/修正/驳回、图片预览/GIF 占位、删除终止态、紧急隐藏/恢复、禁言和全站发布开关。

## 回滚

若启用后出现影响发布的故障：

1. 将生产 `moderation.enabled` 改回 `false` 并重新部署当前版本。
2. 不回退数据库 schema，不删除审核表、版本、图片指纹或业务表内容。
3. 不运行清理或再次迁移，先保留现场并定位问题。
4. 修复后重新执行 `--verify-only`，再按上线步骤启用。

关闭审核后，待审版本不会继续流转；首次先审后发业务行只保留空正文或隐藏状态，不会把待审正文回退暴露。恢复审核后可继续人工处理。

## 日常维护

- 信任等级、处罚、全站控制和紧急隐藏通过管理接口维护；审核规则作为数据库数据维护并在启动时加载；阈值、批次、保留期和稳定提示在 `config/config.yaml` 维护，改动后重启生效。
- 清理 worker 只在审核开启时运行；它保护当前物化、最后通过、待审和仍有操作日志引用的版本。
- 图片审核记录按 `last_used_at` 和 `approval_retention_days` 清理。每次复用已通过图片都会刷新最后访问时间。
- 生产始终使用 `enforce`；`observe` 仅限本地调试。
