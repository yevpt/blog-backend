# 图片上传链路收尾加固 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 multipart 一次性流认证重试、跨碎语图片复用和单文件接口 part 数量遗漏，并恢复双端完整验证。

**Architecture:** 前端在消费流前验证并刷新 access token，且对 multipart 禁止发送后的透明重试。后端把已有碎语图片的归属范围收紧到当前 moment，并在三个单文件 handler 复用统一文件数保护。

**Tech Stack:** Next.js、TypeScript、Vitest、Go 1.25、Gin、testify、gomock。

## Global Constraints

- multipart 必须保持流式转发，不缓存或复制完整请求体。
- 普通可重放请求保留 401 刷新重试；multipart 最多发送一次。
- 碎语图片 key 必须属于当前用户和当前碎语。
- 单文件接口所有字段合计最多一个文件 part。
- 不覆盖工作区中用户已有的审核规则测试改动。

---

### Task 1: 前端 multipart 在消费流前完成认证

**Files:**
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-frontend/apps/web/lib/backend-proxy.ts`
- Test: `/Volumes/External/SynologyDrive/Codes/Blog/blog-frontend/apps/web/lib/backend-proxy.test.ts`

**Interfaces:**
- Consumes: `isAccessTokenValid(token: string | undefined): boolean`、`refreshFromRequest(req)`。
- Produces: `proxyWithRefresh` 的 `retryOnUnauthorized` 选项；`proxyPostForm` 使用不可重放模式。

- [ ] **Step 1: 写过期 token 上传前刷新和 multipart 401 不重试测试**

```ts
it("proxyPostForm 在 access token 过期时先续期再发送一次请求体", async () => {
  // refresh fetch 返回新 token，第二次 fetch 返回上传成功；断言上传只出现一次。
});

it("proxyPostForm 收到后端 401 时不复用请求流", async () => {
  // 使用结构有效 token，后端返回 401；断言状态为 401 且 fetch 仅调用一次。
});
```

- [ ] **Step 2: 运行定向测试并确认 RED**

Run: `pnpm exec vitest --run apps/web/lib/backend-proxy.test.ts`
Expected: 过期 token 测试发送旧 token 后才刷新；401 测试发生第二次 fetch 或返回 502。

- [ ] **Step 3: 最小实现预刷新与不可重放选项**

```ts
import { isAccessTokenValid } from "@/lib/auth-refresh";

function token(req: NextRequest) {
  const value = req.cookies.get(ACCESS_TOKEN_COOKIE)?.value;
  return isAccessTokenValid(value) ? value : undefined;
}

async function proxyWithRefresh(
  req: NextRequest,
  opts: { requireAuth: boolean; retryOnUnauthorized?: boolean },
  fetchBackend: (...) => Promise<Response>,
) {
  const retryOnUnauthorized = opts.retryOnUnauthorized ?? true;
  // 仅在 retryOnUnauthorized 为 true 时执行后端 401 后重试。
}

return proxyWithRefresh(req, { requireAuth: true, retryOnUnauthorized: false }, fetchForm);
```

- [ ] **Step 4: 运行定向测试并确认 GREEN**

Run: `pnpm exec vitest --run apps/web/lib/backend-proxy.test.ts`
Expected: 测试文件全部通过。

- [ ] **Step 5: 提交前端修复**

```bash
git add apps/web/lib/backend-proxy.ts apps/web/lib/backend-proxy.test.ts
git commit -m "fix(proxy): 修复 multipart 续期复用请求流"
```

### Task 2: 收紧普通和审核碎语图片归属

**Files:**
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/service/moment/image.go`
- Test: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/service/moment/moment_test.go`

**Interfaces:**
- Consumes: `momentImageObjectBelongsToMoment(key string, userID, momentID uint) bool`。
- Produces: `existingMomentImage(..., userID, momentID, seq uint)`；审核 URL 校验使用请求中的 `req.ID`。

- [ ] **Step 1: 写普通与审核跨碎语拒绝测试**

```go
func TestMomentService_Save_RejectsReusingSameUsersOtherMomentImage(t *testing.T) {
    // 编辑 ID=9 时提交 moments/7/8/other.jpg，断言 ErrMomentImageInvalid。
}

func TestMomentModeratedCreateRejectsExistingMomentImage(t *testing.T) {
    // 新建审核碎语提交 moments/7/8/old.jpg，断言 ErrMomentImageInvalid 且不调用 Submit。
}

func TestMomentModeratedEditRejectsOtherMomentImage(t *testing.T) {
    // 编辑 ID=9 提交 moments/7/8/old.jpg，断言 ErrMomentImageInvalid。
}
```

- [ ] **Step 2: 运行 service 测试并确认 RED**

Run: `go test ./internal/service/moment -run 'CrossMoment|OtherMoment|Moderated(Create|Edit)'`
Expected: 新增测试因现有逻辑只检查 `moments/{userID}/` 而失败。

- [ ] **Step 3: 最小实现当前碎语归属校验**

```go
func existingMomentImage(ctx context.Context, store storage.ObjectStore, rawURL string, userID, momentID, seq uint) (model.Media, error) {
    key := momentImageObjectKey(rawURL)
    if !momentImageObjectBelongsToMoment(key, userID, momentID) {
        return model.Media{}, ErrMomentImageInvalid
    }
    // 保留存在性检查和 Media 构造。
}

func reusableModerationMomentImage(key string, authorID uint, momentID *uint) bool {
    return momentID != nil && momentImageObjectBelongsToMoment(key, authorID, *momentID)
}
```

- [ ] **Step 4: 更新原审核测试为当前碎语编辑语义并确认 GREEN**

Run: `go test ./internal/service/moment`
Expected: package 全部通过。

- [ ] **Step 5: 提交碎语归属修复**

```bash
git add internal/service/moment/image.go internal/service/moment/moment_test.go
git commit -m "fix(moment): 禁止跨碎语复用图片对象"
```

### Task 3: 补齐单文件接口 part 数量保护

**Files:**
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/handler/friendlink/friendlink.go`
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/handler/music/music.go`
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/handler/moderation/rule_download.go`
- Test: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/handler/friendlink/friendlink_test.go`
- Test: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/handler/music/music_upload_limit_test.go`
- Test: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/handler/moderation/rule_upload_limit_test.go`

**Interfaces:**
- Consumes: `multipartlimit.RejectExcessFileParts(c, 1) bool`。
- Produces: 三个单文件入口统一拒绝两个及以上文件 part。

- [ ] **Step 1: 为三个入口写双文件 part 失败测试**

```go
func TestHandlerRejectsExcessFileParts(t *testing.T) {
    // multipart writer 分别写入目标字段和 extra 字段；调用 handler。
    // 断言响应包含“上传文件过多”，并断言 service 未被调用。
}
```

- [ ] **Step 2: 运行三个 handler package 并确认 RED**

Run: `go test ./internal/handler/friendlink ./internal/handler/music ./internal/handler/moderation -run 'ExcessFileParts'`
Expected: 新增测试因额外文件被静默忽略而失败。

- [ ] **Step 3: 在 FormFile 解析后统一拒绝额外 part**

```go
header, err := c.FormFile("file") // friendlink 使用 "logo"
if multipartlimit.RespondParseError(c, err) {
    return
}
if multipartlimit.RejectExcessFileParts(c, 1) {
    return
}
```

- [ ] **Step 4: 运行 handler package 并确认 GREEN**

Run: `go test ./internal/handler/friendlink ./internal/handler/music ./internal/handler/moderation`
Expected: 三个 package 全部通过。

- [ ] **Step 5: 提交文件数保护**

```bash
git add internal/handler/friendlink/friendlink.go internal/handler/friendlink/friendlink_test.go internal/handler/music/music.go internal/handler/music/music_upload_limit_test.go internal/handler/moderation/rule_download.go internal/handler/moderation/rule_upload_limit_test.go
git commit -m "fix(upload): 补齐单文件接口文件数限制"
```

### Task 4: 修复尺寸测试夹具并完整验证

**Files:**
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/internal/service/user/avatar_normalize_test.go`
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/docs/docs.go`（仅当 swag 生成结果变化）
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/docs/swagger.json`（仅当 swag 生成结果变化）
- Modify: `/Volumes/External/SynologyDrive/Codes/Blog/blog-backend/docs/swagger.yaml`（仅当 swag 生成结果变化）

**Interfaces:**
- Consumes: 当前头像最大尺寸 240px。
- Produces: `testOversizedPNG` 始终生成大于 240px 的图片。

- [ ] **Step 1: 把测试夹具改为 241×241**

```go
func testOversizedPNG(t *testing.T) []byte {
    t.Helper()
    img := image.NewRGBA(image.Rect(0, 0, 241, 241))
    // 保留 PNG 编码。
}
```

- [ ] **Step 2: 验证头像归一化测试**

Run: `go test ./internal/service/user`
Expected: package 全部通过。

- [ ] **Step 3: 重新生成并检查 Swagger**

Run: `make swag && git diff --check`
Expected: 命令成功；若生成文件无语义变化则不纳入提交。

- [ ] **Step 4: 运行双端完整验证**

Run: `go test ./...`
Expected: 后端全部通过。

Run: `pnpm test:run && pnpm check-types`
Expected: 前端全部通过。

- [ ] **Step 5: 提交测试夹具与计划文档**

```bash
git add internal/service/user/avatar_normalize_test.go docs/superpowers/plans/2026-06-30-image-upload-chain-hardening.md
git commit -m "test(upload): 修正头像超限图片测试夹具"
```
