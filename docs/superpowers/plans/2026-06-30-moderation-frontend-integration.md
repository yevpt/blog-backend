# Moderation Frontend Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 API、Admin 和 Web 中接入当前队列、完整审计历史、真实规则导入和逐图展示模式。

**Architecture:** `packages/api` 定义唯一 DTO 和请求；Admin hooks 负责请求状态，组件只渲染；Web 的图片组件只消费后端 `display_mode`。实施必须保留当前工作区已有的碎语发布响应合并和图片加载骨架改动。

**Tech Stack:** TypeScript、React、Next.js、Vitest、Testing Library、workspace API client。

## Global Constraints

- 不覆盖前端当前未提交的 `loading-image`、`moment-image-grid`、`moment-modal`、`use-moment-list` 和 store 改动。
- 队列行 ID 使用 `itemId:revisionId`。
- `superseded` 只在历史展示，文案为“已被新版本替代”。
- Web 不根据碎语整体审核状态猜测图片遮罩。

---

### Task 1: API DTO 与客户端

**Files:**
- Modify: `packages/api/src/types/moderation.ts`
- Modify: `packages/api/src/client.ts`
- Test: `packages/api/src/client.test.ts`

**Interfaces:**
- Produces `AdminModerationHistoryResp`、`AdminModerationHistoryRevisionResp`、`AdminModerationHistoryEventResp`。
- Produces `moderation.getHistory(itemId, req)`。

- [ ] **Step 1: 写失败测试**

断言历史请求 URL、page/page_size，multipart FormData 包含真实 file，错误报告返回 Blob 文件名。

- [ ] **Step 2: 验证 RED**

Run: `pnpm --filter @repo/api test -- client.test.ts`

Expected: FAIL，历史客户端尚不存在。

- [ ] **Step 3: 实现并验证**

```ts
getHistory: (itemId: number, req: { page?: number; page_size?: number } = {}) =>
  fetchAuthed<AdminModerationHistoryResp>(
    `/admin/moderation/items/${itemId}/history${buildQuery(req)}`,
    { method: "GET" },
  )
```

Run: `pnpm --filter @repo/api test && pnpm --filter @repo/api typecheck`，Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add packages/api
git commit -m "feat(api): 增加审核历史与图片响应类型"
```

### Task 2: 管理端队列状态与真实刷新

**Files:**
- Modify: `apps/admin/src/modules/moderation/model.ts`
- Modify: `apps/admin/src/modules/moderation/hooks/use-moderation-list.ts`
- Modify: `apps/admin/src/modules/moderation/components/ModerationQueuePanel.tsx`
- Test: `apps/admin/src/modules/moderation/model.test.ts`
- Test: `apps/admin/src/modules/moderation/hooks/use-moderation-list.test.ts`
- Test: `apps/admin/src/modules/moderation/ModerationPage.test.tsx`

- [ ] **Step 1: 写失败测试**

断言 superseded 文案、桌面/移动端复合 key、调用 `await refetch()` 时 Promise 仅在新 HTTP 请求结束后 resolve。

- [ ] **Step 2: 验证 RED**

Run: `pnpm --filter admin test -- moderation`

Expected: FAIL，当前 refetch 只增加 token 且行 key 只有 itemId。

- [ ] **Step 3: 实现**

`useModerationList` 使用递增 request sequence 和 resolver ref 保存刷新 Promise；load 完成在 finally resolve 对应请求。`ModerationRow` 增加 `rowId: \`${itemId}:${revisionId}\``，两端列表都使用 rowId。

- [ ] **Step 4: 验证并提交**

Run: `pnpm --filter admin test -- moderation && pnpm --filter admin typecheck`，Expected: PASS。

```bash
git add apps/admin/src/modules/moderation
git commit -m "fix(admin): 修正审核队列状态与刷新时序"
```

### Task 3: 审核详情审计历史页签

**Files:**
- Create: `apps/admin/src/modules/moderation/hooks/use-moderation-history.ts`
- Create: `apps/admin/src/modules/moderation/components/ModerationHistory.tsx`
- Create: `apps/admin/src/modules/moderation/components/ModerationImageGallery.tsx`
- Modify: `apps/admin/src/modules/moderation/components/ModerationReviewDialog.tsx`
- Modify: `apps/admin/src/modules/moderation/components/ModerationReviewDetails.tsx`
- Test: `apps/admin/src/modules/moderation/ModerationPage.test.tsx`
- Test: `apps/admin/src/modules/moderation/components/ModerationHistory.test.tsx`

- [ ] **Step 1: 写失败测试**

打开详情后断言“当前内容/审计历史”页签；切换后加载 revision、图片、操作人、理由和时间；下一页请求正确；私有图片只使用 access_url。

- [ ] **Step 2: 验证 RED**

Run: `pnpm --filter admin test -- ModerationHistory ModerationPage`

Expected: FAIL，组件尚不存在。

- [ ] **Step 3: 实现**

hook 只在弹窗打开且选中历史页签时请求；组件按 revision 分组渲染事件时间线和图片网格，`superseded` 显示“已被新版本替代”。当前页继续保留审核动作。

- [ ] **Step 4: 验证并提交**

Run: `pnpm --filter admin test -- moderation && pnpm --filter admin typecheck`，Expected: PASS。

```bash
git add apps/admin/src/modules/moderation
git commit -m "feat(admin): 增加审核审计历史视图"
```

### Task 4: 规则导入与状态展示

**Files:**
- Modify: `apps/admin/src/modules/moderation/rules/components/RuleImportDialog.tsx`
- Modify: `apps/admin/src/modules/moderation/rules/hooks/use-rule-imports.ts`
- Modify: `apps/admin/src/modules/moderation/rules/model.ts`
- Test: `apps/admin/src/modules/moderation/rules/components/RuleImportDialog.test.tsx`
- Test: `apps/admin/src/modules/moderation/rules/hooks/use-rule-imports.test.ts`
- Test: `apps/admin/src/modules/moderation/rules/model.test.ts`

- [ ] **Step 1: 写失败测试**

上传包含 regexp/composite 的 CSV File，断言客户端收到原 File；验证 invalid 错误报告下载、ready 发布确认、默认列表一启用一停用。

- [ ] **Step 2: 验证 RED**

Run: `pnpm --filter admin test -- moderation/rules`

Expected: 至少规则状态投影用例 FAIL；导入用例用于锁定真实 multipart 行为。

- [ ] **Step 3: 实现并验证**

显示 CSV 完整字段说明和 TXT 限制；上传前校验扩展名、空文件和配置允许的前端上限，后端仍是最终边界。Run: `pnpm --filter admin test -- moderation/rules && pnpm --filter admin typecheck`，Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add apps/admin/src/modules/moderation/rules
git commit -m "fix(admin): 补齐规则导入与状态展示"
```

### Task 5: Web 逐图展示模式

**Files:**
- Modify: `apps/web/components/moments/types.ts`
- Modify: `apps/web/components/moments/moment-image-grid.tsx`
- Modify: `apps/web/components/moderation/index.tsx`
- Test: `apps/web/components/moments/moment-image-grid.test.tsx`
- Preserve and extend: 当前未提交的 `apps/web/components/common/loading-image*`、`moment-modal*`、`use-moment-list*`、`use-moment-modal*`、`enrich-moment-from-publish*`。

- [ ] **Step 1: 写失败测试**

同一碎语同时传入 original、blurred、gif_placeholder 三张图，断言各自按 display_mode 渲染；内容有 pending revision 时 original 仍展示原图；作者 pending_images 保持可编辑回显。

- [ ] **Step 2: 验证 RED**

Run: `pnpm --filter web test -- moment-image-grid`

Expected: FAIL，当前仍存在整体状态影响图片展示的路径。

- [ ] **Step 3: 合并现有用户改动并实现**

保留 `DeferredNativeImage.layout`、发布响应补齐和 loading frame；只把图片源选择收口为 `display_mode` 分支，不回退这些未提交改动。

- [ ] **Step 4: 验证并提交**

Run: `pnpm --filter web test -- moment-image-grid loading-image use-moment-list use-moment-modal && pnpm --filter web typecheck`，Expected: PASS。

提交前只暂存本任务实际改动；用户原有未提交文件若无法区分则先保留为工作区改动，不擅自纳入提交。

### Task 6: 前端全量验证

- [ ] Run: `pnpm --filter @repo/api test && pnpm --filter admin test && pnpm --filter web test`
- [ ] Run: `pnpm --filter @repo/api typecheck && pnpm --filter admin typecheck && pnpm --filter web typecheck`
- [ ] Run: `pnpm --filter admin build && pnpm --filter web build`

Expected: 全部 PASS；若存在与本任务无关的基线失败，记录精确测试名并与基线对比。
