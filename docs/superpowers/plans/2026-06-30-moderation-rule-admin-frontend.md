# Moderation Rule Admin Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add typed rule-management API clients and a responsive “规则管理” tab with cursor navigation, rule editing, text testing, templates, exports, and recoverable bulk-import workflows.

**Architecture:** `packages/api` owns exact transport types, FormData calls, and authenticated Blob downloads. `apps/admin` owns URL-backed filters, a cursor stack, task polling, dialogs, and responsive presentation. Existing moderation queue/control/user behavior remains unchanged.

**Tech Stack:** React 19, TypeScript 6, Vite 8, pnpm 11, Vitest 4, Testing Library, TailwindCSS, `@repo/api`, `@repo/ui`, `@repo/icons`.

## Global Constraints

- Complete the backend foundation and management API plans first.
- Before implementation read frontend skills `extending-api`, `building-admin-module`, `building-ui`, and `writing-tests` from `../blog-frontend/.agents/skills`.
- Preserve current unrelated dirty changes under `apps/web`; this plan only changes `packages/api` and `apps/admin/src/modules/moderation` plus one admin download helper.
- Do not use `any`; use exact unions, discriminated states, and `unknown` error narrowing.
- Every changed Hook gets `*.test.ts`; every changed component/page gets `*.test.tsx`.
- Admin is a client SPA: do not add Next.js directives or server actions.
- Mobile lists must remain operable; do not clip a desktop table on small screens.
- Never load all rules into the browser; list limit is at most 100.

## File Structure

- `packages/api/src/types/moderation-rule.ts`: rule, metadata, ruleset, import, and test contracts.
- `packages/api/src/client.ts`: JSON, FormData, and Blob moderation rule methods.
- `apps/admin/src/modules/moderation/rules/model.ts`: labels, validation, query conversion, and row mapping.
- `apps/admin/src/modules/moderation/rules/hooks/*`: list/status and import workflows.
- `apps/admin/src/modules/moderation/rules/components/*`: focused panels and dialogs.
- `apps/admin/src/lib/download.ts`: safe browser Blob download/revocation helper.

---

### Task 1: Typed Rule API and Authenticated Downloads

**Files:**
- Create: `../blog-frontend/packages/api/src/types/moderation-rule.ts`
- Modify: `../blog-frontend/packages/api/src/index.ts`
- Modify: `../blog-frontend/packages/api/src/client.ts`
- Modify: `../blog-frontend/packages/api/src/client.test.ts`

**Interfaces:**
- Produces: every `AdminModerationRule*` and `AdminModerationRuleImport*` type.
- Produces: `apiClient.moderation.rules.*` and `apiClient.moderation.ruleImports.*`.

- [ ] **Step 1: Write failing client tests for cursor queries, FormData, and Blob responses**

```ts
it("规则列表只发送明确提供的游标筛选", async () => {
  await client.moderation.rules.list({ cursor: 42, limit: 50, search_mode: "prefix", pattern: "风险" });
  expect(fetchMock).toHaveBeenCalledWith(
    "http://api/admin/moderation/rules?cursor=42&limit=50&search_mode=prefix&pattern=%E9%A3%8E%E9%99%A9",
    expect.objectContaining({ method: "GET" }),
  );
});

it("规则导入使用 FormData 且不覆盖 multipart boundary", async () => {
  const file = new File(["词条"], "rules.txt", { type: "text/plain" });
  await client.moderation.ruleImports.create({ file, source_name: "采购词库-2026", category: "other", risk_level: "medium", effect: "review", priority: 100 });
  const init = vi.mocked(fetch).mock.calls.at(-1)?.[1];
  expect(init?.body).toBeInstanceOf(FormData);
  expect(new Headers(init?.headers).has("Content-Type")).toBe(false);
});
```

- [ ] **Step 2: Run API tests and verify failure**

Run from `../blog-frontend`: `pnpm --filter @repo/api test`

Expected: FAIL because rule types and methods do not exist.

- [ ] **Step 3: Define exact transport types**

```ts
export type ModerationRuleType = "keyword" | "regexp" | "composite";
export type ModerationRuleEffect = "review" | "allow";
export type ModerationRuleCategory = "politics" | "pornography" | "violence" | "terrorism" | "gambling" | "drugs" | "fraud" | "abuse" | "advertising" | "minors" | "other";
export type ModerationRulesetStatus = "building" | "ready" | "publishing" | "published" | "failed" | "superseded";
export type ModerationImportValidationStatus = "queued" | "validating" | "valid" | "invalid" | "canceled";

export interface AdminModerationRuleListReq {
  cursor?: number; limit?: number; id?: number; pattern?: string;
  search_mode?: "exact" | "prefix"; category?: ModerationRuleCategory;
  rule_type?: ModerationRuleType; risk_level?: ModerationRiskLevel;
  effect?: ModerationRuleEffect; source_id?: number; active?: boolean;
}

export interface AdminModerationRulePageResp {
  list: AdminModerationRuleResp[];
  next_cursor: number;
  has_more: boolean;
}
```

```ts
export interface AdminModerationRuleSaveReq {
  expected_ruleset_version: number; name?: string; rule_type: ModerationRuleType;
  pattern: string; category: ModerationRuleCategory; effect: ModerationRuleEffect;
  risk_level: ModerationRiskLevel; priority: number; source_id: number;
}
export interface AdminModerationRuleBatchStatusReq {
  expected_ruleset_version: number; rule_ids: number[]; active: boolean;
}
export interface AdminModerationRuleJobResp { ruleset_id: number; base_ruleset_id: number; status: ModerationRulesetStatus; }
export interface AdminModerationRuleTestReq { text: string; ruleset_id?: number; }
export interface AdminModerationRuleTestResp {
  ruleset_id: number; risk_level: ModerationRiskLevel; truncated: boolean;
  matches: AdminModerationRuleTestHit[]; suppressed: AdminModerationRuleTestHit[];
}
export interface AdminModerationRuleImportCreateReq {
  file: File; source_name: string; category: ModerationRuleCategory;
  risk_level: ModerationRiskLevel; effect: ModerationRuleEffect; priority: number;
}
export interface AdminModerationRuleImportPageResp {
  list: AdminModerationRuleImportResp[]; next_cursor: number; has_more: boolean;
}
export interface BinaryDownload { blob: Blob; filename?: string; }

export interface AdminModerationRuleResp {
  id: number; name?: string; rule_type: ModerationRuleType; pattern: string;
  category: ModerationRuleCategory; effect: ModerationRuleEffect;
  risk_level: ModerationRiskLevel; priority: number; source_id: number;
  activated_ruleset_id: number; deactivated_ruleset_id?: number; active: boolean;
  replaces_rule_id?: number; created_at: string; updated_at: string;
}
export interface AdminModerationRuleSourceResp { id: number; name: string; }
export interface AdminModerationRuleMetadataResp {
  categories: Array<{ key: ModerationRuleCategory; label: string }>;
  rule_types: Array<{ key: ModerationRuleType; label: string }>;
  effects: Array<{ key: ModerationRuleEffect; label: string }>;
  risk_levels: Array<{ key: ModerationRiskLevel; label: string }>;
  sources: AdminModerationRuleSourceResp[];
}
export interface AdminModerationRuleStatusResp {
  current_ruleset_id: number; candidate_ruleset_id?: number;
  candidate_status?: ModerationRulesetStatus; rule_count: number;
  keyword_count: number; regexp_count: number; composite_count: number;
  index_bytes: number; build_peak_bytes: number; published_at: string;
}
export interface AdminModerationRuleTestHit {
  rule_id: number; name?: string; pattern: string; category: ModerationRuleCategory;
  effect: ModerationRuleEffect; risk_level: ModerationRiskLevel; excerpt: string;
}
export interface AdminModerationRuleImportResp {
  id: number; file_name: string; format: "csv" | "txt"; source_id: number;
  validation_status: ModerationImportValidationStatus; ruleset_id?: number;
  ruleset_status?: ModerationRulesetStatus; total_rows: number; valid_rows: number;
  duplicate_rows: number; error_rows: number; index_bytes?: number;
  build_peak_bytes?: number; created_at: string; updated_at: string;
}
export interface AdminModerationRuleImportPublishReq { expected_ruleset_version: number; }
```

Export all types from `src/index.ts`.

- [ ] **Step 4: Add authenticated binary request support without changing JSON behavior**

Factor the token/refresh retry so both JSON and binary requests use the same 401 flow. Binary success returns `response.blob()` and parses RFC 5987 or quoted `Content-Disposition` filenames; binary errors parse the backend JSON envelope into `ApiError`.

- [ ] **Step 5: Add nested client methods**

```ts
moderation: {
  rules: {
    list, metadata, status, create, replace, batchStatus, testText, export: exportRules,
  },
  ruleImports: {
    list, get, create, publish, cancel, template, errors,
  },
  // existing item/control/user methods remain unchanged
}
```

- [ ] **Step 6: Run API tests, types, and lint**

Run from `../blog-frontend`:

```bash
pnpm --filter @repo/api test
pnpm --filter @repo/api check-types
pnpm --filter @repo/api lint
```

Expected: PASS.

- [ ] **Step 7: Commit the API client**

```bash
git -C ../blog-frontend add packages/api
git -C ../blog-frontend commit -m "feat(api): 新增审核规则管理客户端"
```

### Task 2: Rule Model, URL Filters, and Cursor State Hook

**Files:**
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/model.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/model.test.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-list.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-list.test.ts`
- Modify: `../blog-frontend/apps/admin/src/test/render-with-admin-router.tsx`

**Interfaces:**
- Produces: `RuleRow`, `RuleFilters`, validation/conversion helpers, and `UseRuleListResult`.
- Consumes: Task 1 API client.

- [ ] **Step 1: Write failing model and Hook tests**

```ts
it("筛选变化会清空游标栈并从第一批重新加载", async () => {
  const { result } = renderHookWithAdminRouter(() => useRuleList());
  await waitFor(() => expect(result.current.rows).toHaveLength(2));
  act(() => result.current.nextPage());
  act(() => result.current.setFilter("category", "fraud"));
  await waitFor(() => expect(apiClient.moderation.rules.list).toHaveBeenLastCalledWith(
    expect.objectContaining({ category: "fraud", cursor: undefined }),
  ));
  expect(result.current.canGoPrevious).toBe(false);
});
```

- [ ] **Step 2: Run focused admin tests and verify failure**

Run from `../blog-frontend`: `pnpm --filter admin test -- src/modules/moderation/rules`

Expected: FAIL because the model and Hook do not exist.

- [ ] **Step 3: Implement pure mapping and validation**

```ts
export interface RuleFormValues {
  name: string; ruleType: ModerationRuleType; pattern: string;
  category: ModerationRuleCategory; effect: ModerationRuleEffect;
  riskLevel: ModerationRiskLevel; priority: string; sourceId: string;
}

export function validateRuleForm(values: RuleFormValues): RuleFormErrors {
  const errors: RuleFormErrors = {};
  if (!values.pattern.trim()) errors.pattern = "请输入匹配内容";
  if (values.effect === "allow" && values.ruleType !== "keyword") errors.effect = "白名单仅支持关键词";
  const priority = Number(values.priority);
  if (!Number.isInteger(priority)) errors.priority = "优先级必须是整数";
  return errors;
}
```

Add backend-key label maps, rule-to-row conversion, filter-to-query conversion, and exact/prefix search validation.

- [ ] **Step 4: Implement URL-backed filters plus an in-memory cursor stack**

Filters belong in URL search params; cursors do not, because a cursor is valid only for one filter snapshot. `nextPage` pushes the current cursor, `previousPage` pops, and every filter/limit change resets the stack. Abort stale requests or ignore responses whose request sequence is not current.

- [ ] **Step 5: Run model and Hook tests**

Run from `../blog-frontend`: `pnpm --filter admin test -- src/modules/moderation/rules/model.test.ts src/modules/moderation/rules/hooks/use-rule-list.test.ts`

Expected: PASS.

- [ ] **Step 6: Commit list state**

```bash
git -C ../blog-frontend add apps/admin/src/modules/moderation/rules apps/admin/src/test/render-with-admin-router.tsx
git -C ../blog-frontend commit -m "feat(admin): 新增审核规则游标状态"
```

### Task 3: Status, Filters, Desktop Table, and Mobile Cards

**Files:**
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-status.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-status.test.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleStatusSummary.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleToolbar.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleTable.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleMobileList.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RulePanel.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RulePanel.test.tsx`

**Interfaces:**
- Produces: `RulePanel` with action callbacks; it does not own dialogs.
- Consumes: Tasks 1–2 metadata/list/status APIs and Hooks.

- [ ] **Step 1: Write failing panel tests for loading, error, filters, and cursor navigation**

```tsx
it("显示索引内存、筛选器和下一批操作", async () => {
  render(<RulePanel {...fixtureProps({ hasMore: true, indexBytes: 64 * 1024 * 1024 })} />);
  expect(screen.getByText("64 MB")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "下一批" })).toBeEnabled();
  expect(screen.getByLabelText("规则分类")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run panel tests and verify failure**

Run from `../blog-frontend`: `pnpm --filter admin test -- src/modules/moderation/rules/components/RulePanel.test.tsx`

Expected: FAIL because components do not exist.

- [ ] **Step 3: Implement status and metadata loading**

`useRuleStatus` loads status and metadata together, polls every two seconds only while a candidate is building/publishing, and exposes `reload`. Clear the interval in the effect cleanup.

- [ ] **Step 4: Implement responsive rule presentation**

Desktop columns: ID/name+pattern, category/source, type/effect, risk, priority, current state, actions. Mobile cards present the same values and edit/status buttons. Toolbar exposes search mode, search value, indexed filters, reset, add, test, import, template, and export actions.

- [ ] **Step 5: Run panel, Hook, type, and lint tests**

Run from `../blog-frontend`:

```bash
pnpm --filter admin test -- src/modules/moderation/rules
pnpm --filter admin check-types
pnpm --filter admin lint
```

Expected: PASS.

- [ ] **Step 6: Commit the responsive panel**

```bash
git -C ../blog-frontend add apps/admin/src/modules/moderation/rules
git -C ../blog-frontend commit -m "feat(admin): 新增审核规则列表面板"
```

### Task 4: Rule Form and Batch Status Actions

**Files:**
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-mutations.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-mutations.test.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleFormDialog.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleFormDialog.test.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleBatchBar.tsx`

**Interfaces:**
- Produces: create, replace, enable, disable actions with conflict-safe UI state.
- Consumes: current ruleset ID from status and selected rule IDs from panel.

- [ ] **Step 1: Write failing validation, immutable edit, and conflict tests**

```ts
it("编辑提交携带当前规则集版本且不修改旧 ID", async () => {
  const { result } = renderHookWithAdminRouter(() => useRuleMutations({ rulesetId: 7, onCompleted }));
  await act(() => result.current.replace(41, validFormValues()));
  expect(apiClient.moderation.rules.replace).toHaveBeenCalledWith(41, expect.objectContaining({ expected_ruleset_version: 7 }));
});

it("版本冲突保留表单并提示刷新", async () => {
  vi.mocked(apiClient.moderation.rules.replace).mockRejectedValue(new ApiError("MODERATION_RULESET_CONFLICT", "版本已变化"));
  // submit and assert dialog remains open with conflict copy
});
```

- [ ] **Step 2: Run mutation/dialog tests and verify failure**

Run from `../blog-frontend`: `pnpm --filter admin test -- src/modules/moderation/rules/hooks/use-rule-mutations.test.ts src/modules/moderation/rules/components/RuleFormDialog.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement create/edit form behavior**

Reset from props only when opening. Disable effect=`allow` for non-keywords, display backend memory/regex errors inline, and keep entered values on conflict. Successful asynchronous jobs close the form, show a toast, and trigger status polling rather than pretending the rule is already active.

- [ ] **Step 4: Implement bounded multi-select status actions**

The batch bar shows selected count, rejects more than 1000 locally, asks for confirmation, sends the expected version, then clears selection only after the job is accepted. Disable batch actions while another candidate is ready/building.

- [ ] **Step 5: Run mutation, component, types, and lint checks**

Run from `../blog-frontend`:

```bash
pnpm --filter admin test -- src/modules/moderation/rules
pnpm --filter admin check-types
pnpm --filter admin lint
```

Expected: PASS.

- [ ] **Step 6: Commit mutations**

```bash
git -C ../blog-frontend add apps/admin/src/modules/moderation/rules
git -C ../blog-frontend commit -m "feat(admin): 新增审核规则编辑与批量启停"
```

### Task 5: Current/Candidate Text Test Dialog

**Files:**
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-test.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-test.test.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleTestDialog.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleTestDialog.test.tsx`

**Interfaces:**
- Produces: bounded admin test workflow for current or one ready candidate.

- [ ] **Step 1: Write failing current/candidate and truncation tests**

```tsx
it("候选就绪时允许选择候选版本并展示截断提示", async () => {
  const user = userEvent.setup();
  render(<RuleTestDialog open status={readyCandidateStatus()} onClose={vi.fn()} />);
  await user.selectOptions(screen.getByLabelText("测试规则集"), "candidate");
  await user.type(screen.getByLabelText("测试文本"), "测试内容");
  await user.click(screen.getByRole("button", { name: "开始测试" }));
  expect(await screen.findByText("还有命中未展示")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run tests and verify failure**

Run from `../blog-frontend`: `pnpm --filter admin test -- src/modules/moderation/rules/hooks/use-rule-test.test.ts src/modules/moderation/rules/components/RuleTestDialog.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement bounded test state and result rendering**

Enforce 10000 characters locally, submit current or candidate ruleset ID, and render final risk, version, matched rules, highlighted excerpt, suppressed allow hits, and truncation state. Do not persist test text in URL or local storage.

- [ ] **Step 4: Run tests, types, and lint**

Run from `../blog-frontend`: `pnpm --filter admin test -- src/modules/moderation/rules && pnpm --filter admin check-types && pnpm --filter admin lint`

Expected: PASS.

- [ ] **Step 5: Commit text testing**

```bash
git -C ../blog-frontend add apps/admin/src/modules/moderation/rules
git -C ../blog-frontend commit -m "feat(admin): 新增审核规则文本试跑"
```

### Task 6: Import Wizard, History, Templates, and Downloads

**Files:**
- Create: `../blog-frontend/apps/admin/src/lib/download.ts`
- Create: `../blog-frontend/apps/admin/src/lib/download.test.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-imports.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/hooks/use-rule-imports.test.ts`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleImportDialog.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleImportDialog.test.tsx`
- Create: `../blog-frontend/apps/admin/src/modules/moderation/rules/components/RuleImportHistory.tsx`

**Interfaces:**
- Produces: upload/validation/build/ready/publish/cancel/history UI and safe Blob downloads.

- [ ] **Step 1: Write failing download cleanup and task-resume tests**

```ts
it("下载完成后撤销临时 URL", () => {
  downloadBlob(new Blob(["x"]), "rules.csv");
  expect(URL.revokeObjectURL).toHaveBeenCalledWith(expect.stringMatching(/^blob:/));
});

it("关闭并重新打开时从后端恢复活动任务", async () => {
  const { result, rerender } = renderRuleImportsHook({ open: true });
  await waitFor(() => expect(result.current.active?.id).toBe(9));
  rerender({ open: false });
  rerender({ open: true });
  expect(apiClient.moderation.ruleImports.get).toHaveBeenCalledWith(9);
});
```

- [ ] **Step 2: Run import/download tests and verify failure**

Run from `../blog-frontend`: `pnpm --filter admin test -- src/lib/download.test.ts src/modules/moderation/rules/hooks/use-rule-imports.test.ts src/modules/moderation/rules/components/RuleImportDialog.test.tsx`

Expected: FAIL.

- [ ] **Step 3: Implement safe download helper**

Create an object URL, attach an anchor with `download`, click, remove it, and revoke the URL in `finally`. Use backend filename when present; otherwise use stable Chinese-free filenames for cross-browser compatibility.

- [ ] **Step 4: Implement import state and polling lifecycle**

Poll every two seconds only for queued/validating/building/publishing states; clear interval on terminal state and unmount. Keep active task ID in component state, not persistent storage. Reopening queries import history and restores the newest non-terminal task. Never keep the uploaded `File` after the create request resolves.

- [ ] **Step 5: Implement six-step wizard and history**

Steps: template/file, defaults/source, upload, validation/build statistics, optional candidate test, publish. Source is a required 1–100 character name; the backend reuses or creates it and CSV rows cannot override it. Show total/valid/duplicate/error counts, nodes, index/peak memory, error download, cancel, expected-version conflict, and terminal result. History consumes `AdminModerationRuleImportPageResp.next_cursor` with the same cursor-stack rule as the rule list and never loads all tasks.

- [ ] **Step 6: Wire template, error, and export downloads**

Template menu calls CSV/TXT endpoints; error button uses task ID; export applies current filters. Disable export while no published ruleset exists and surface `ApiError` messages through the existing toast queue.

- [ ] **Step 7: Run import, download, type, and lint checks**

Run from `../blog-frontend`:

```bash
pnpm --filter admin test -- src/lib/download.test.ts src/modules/moderation/rules
pnpm --filter admin check-types
pnpm --filter admin lint
```

Expected: PASS.

- [ ] **Step 8: Commit import UI**

```bash
git -C ../blog-frontend add apps/admin/src/lib/download.ts apps/admin/src/lib/download.test.ts apps/admin/src/modules/moderation/rules
git -C ../blog-frontend commit -m "feat(admin): 新增审核规则导入与下载"
```

### Task 7: Moderation Page Integration and Frontend Acceptance

**Files:**
- Modify: `../blog-frontend/apps/admin/src/modules/moderation/ModerationPage.tsx`
- Modify: `../blog-frontend/apps/admin/src/modules/moderation/ModerationPage.test.tsx`
- Modify: `../blog-frontend/apps/admin/src/modules/moderation/module.tsx`

**Interfaces:**
- Consumes: all prior frontend tasks.
- Produces: the final “规则管理” tab without regressing queue/control/user tabs.

- [ ] **Step 1: Write failing page integration tests**

```tsx
it("内容审核页提供规则管理标签且不提前请求隐藏标签数据", async () => {
  const user = userEvent.setup();
  renderModerationPage();
  expect(screen.getByRole("tab", { name: "规则管理" })).toBeInTheDocument();
  expect(apiClient.moderation.rules.list).not.toHaveBeenCalled();
  await user.click(screen.getByRole("tab", { name: "规则管理" }));
  await waitFor(() => expect(apiClient.moderation.rules.list).toHaveBeenCalledTimes(1));
});
```

- [ ] **Step 2: Run page test and verify failure**

Run from `../blog-frontend`: `pnpm --filter admin test -- src/modules/moderation/ModerationPage.test.tsx`

Expected: FAIL because the tab is absent.

- [ ] **Step 3: Integrate the rule tab lazily**

Add `TabsItem id="rules"` and render `RulePanel` only after the tab has first been selected, so opening the moderation queue does not start rule polling. Update page/module descriptions to include rule management without changing the route.

- [ ] **Step 4: Run focused and full frontend verification**

Run from `../blog-frontend`:

```bash
pnpm --filter @repo/api test
pnpm --filter admin test
pnpm --filter @repo/api check-types
pnpm --filter admin check-types
pnpm --filter @repo/api lint
pnpm --filter admin lint
pnpm --filter admin build
```

Expected: all commands PASS.

- [ ] **Step 5: Verify unrelated user changes remain untouched**

Run: `git -C ../blog-frontend status --short`

Expected: the pre-existing `apps/web` comment/guestbook changes remain present and are neither staged nor modified by this feature.

- [ ] **Step 6: Commit page integration**

```bash
git -C ../blog-frontend add apps/admin/src/modules/moderation
git -C ../blog-frontend commit -m "feat(admin): 接入审核规则管理页面"
```
