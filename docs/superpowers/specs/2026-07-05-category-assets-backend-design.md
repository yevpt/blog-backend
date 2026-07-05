# 分类可选素材上传与持久化：后端设计

## 目标

让分类描述、图标、封面在创建和更新时均为真正可选，并提供安全的 SVG 图标上传与文章封面同级别的位图上传、压缩、临时资源转存、替换和清理能力。

本设计只覆盖 `blog-backend`。管理端交互与 `@repo/api` 适配见 `blog-frontend/docs/superpowers/specs/2026-07-05-category-assets-frontend-design.md`。

## 当前根因

- `dto.CategoryCreateReq` 对 `icon`、`description`、`cover_img_url` 使用 `binding:"required"`。
- `newCategoryFromCreateReq` 再次拒绝三个空值。
- 分类 service 没有对象存储依赖，也没有上传、临时 key 校验或正式资源转存流程。
- 分类响应没有统一把对象 key 解析为可访问 URL。
- 现有 `/uploads/temp` 是位图管线，`imagefile.Validate` 不接受 SVG，不适合直接混入 SVG 分支。

## HTTP 契约

新增两个管理员接口：

```text
POST /admin/categories/uploads/icon
POST /admin/categories/uploads/cover
```

请求均为 multipart，文件字段名 `file`。复用 admin 鉴权和 claims 中的管理员 user ID，不从客户端接收 user ID。

统一响应：

```json
{
  "key": "temp/categories/7/icon/<hash>.svg",
  "url": "https://cdn.example.com/...",
  "size": 1234,
  "mime": "image/svg+xml"
}
```

可新增 `dto.CategoryAssetUploadResp`，字段为 `key/url/size/mime`。

分类创建请求中 `description`、`icon`、`cover_img_url` 改为可选指针并移除 required；名称与 `seq` 保持必填。更新请求沿用指针语义：字段未传表示不修改，传空字符串表示清空。

## 上传设计

### 封面

- 文件读取上限复用 `uploadservice.MaxTempImageBytes`（10 MB）和 multipart body guard。
- 复用 `imagefile.Validate` 与文章 `PrepareForStorage` 参数，最终对象上限复用 `MaxArticleTempImageStoredBytes`（3 MB）。
- 非 GIF 位图按文章管线压缩/转码；GIF 沿用文章限制，不另造参数。
- 临时 key：`temp/categories/{userID}/cover/{md5-or-sha256}.{ext}`。
- 返回对象存储 URL 仅用于预览，保存请求使用 key。

应提取或复用文章图片处理函数/选项，避免复制一套逐渐漂移的压缩参数；若现有函数包内不可见，优先提取到恰当的共享包，而不是在 category 中硬编码副本。

### SVG 图标

建议上限 256 KB，只接受 UTF-8 SVG。客户端的扩展名和 MIME 仅作提示，后端解析结果才是权威。

采用 XML token 白名单，不做危险黑名单：

- 根节点必须是 `svg`，禁止 DOCTYPE、ENTITY 和处理指令。
- 允许常用静态绘图节点：`svg`、`g`、`path`、`rect`、`circle`、`ellipse`、`line`、`polyline`、`polygon`、`title`、`desc`、`defs`、`clipPath`、`mask`、`linearGradient`、`radialGradient`、`stop`、`use`。
- 属性仅允许几何、颜色、描边、透明度、变换、viewBox、尺寸、渐变、裁剪、mask、id、xmlns 等明确集合。
- 禁止任何 `on*` 事件属性、`style`、脚本、`foreignObject`、`iframe`、`object`、`embed`、`image`、音视频及未知节点。
- `href`/`xlink:href` 仅允许当前文档内的 `#id`；`fill`、`stroke`、`clip-path`、`mask` 中的 `url(...)` 也只能引用本地 fragment。
- 限制 XML 深度、节点数、属性数和文本长度，避免解析器资源耗尽。
- 校验通过后由解析结果重新编码规范化 SVG，不原样信任上传字节。

临时 key：`temp/categories/{userID}/icon/{sha256}.svg`，Content-Type 固定为 `image/svg+xml`。相同内容幂等复用对象。

## 保存与资源归一化

Category service 注入 `storage.ObjectStore`，创建/更新方法增加 `context.Context` 与当前管理员 `userID`。handler 从 JWT claims 获取 user ID，和文章保存方式一致。

允许的素材引用：

- 当前管理员的 `temp/categories/{userID}/icon|cover/` key 或本站 URL。
- 当前分类已有的 `categories/{categoryID}/icon|cover/` key 或本站 URL。
- 同一分类对应目录下确实存在的正式对象。

拒绝外链、其他用户临时 key、其他分类正式 key和不存在的对象。

正式目录：

```text
categories/{categoryID}/icon/<filename>
categories/{categoryID}/cover/<filename>
```

### 创建

创建需要先取得分类 ID。参考音乐 `SaveAlbum` 的 prepare callback，在 repository 的数据库事务内：

1. 先插入不含正式素材引用的分类，取得 ID。
2. prepare callback 校验/复制可选临时素材到正式目录。
3. 更新分类的正式 key 后提交事务。
4. 任一素材失败则回滚数据库事务，并尽力删除本次新复制的正式对象。
5. 成功后尽力删除临时对象；清理失败记录日志，不把已成功保存伪装成请求失败。

### 更新

先读取旧素材 key。对请求中出现的字段分别处理：

- 未传：保持原值。
- 空字符串：数据库写 NULL，保存成功后清理旧正式对象。
- 新临时 key：复制为正式 key，数据库更新成功后删除临时对象和被替换的旧正式对象。
- 原正式 key/本站 URL：确认存在且属于该分类后保持。

数据库失败时删除本次新复制的正式对象；旧资源只能在数据库成功后删除。

### 删除

分类删除成功后，尽力清理 `categories/{id}/icon/` 与 `cover/` 下当前引用对象。对象存储清理失败写日志，不回滚已经完成的数据库删除。

## DTO 与响应

- `CategoryCreateReq.Icon/Description/CoverImgUrl` 改为 `*string` + `omitempty`。
- `newCategoryFromCreateReq` 使用 `strutil.CleanOptional`，只校验名称和 seq。
- 删除不再适用的 `ErrCategoryIconRequired`、`ErrCategoryDescRequired`、`ErrCategoryCoverRequired` 及 handler 映射。
- 列表、创建、更新、删除响应统一通过 `storage.ResolvePtrURL` 将 key 转为 URL；数据库继续只存 key。
- 为兼容历史本站 URL，归一化时先通过 `ObjectKey` 还原 key；不允许新的外链值。

## 分层与文件建议

- handler：multipart 读取、body guard、claims、Swagger、错误到 HTTP 响应映射。
- service/category：上传编排、SVG 校验调用、素材归一化、资源生命周期。
- repository/category：事务和 prepare callback，不接触 HTTP、JWT 或对象存储具体实现。
- `pkg`：只有确实可复用的 SVG 静态安全校验器或共享图片处理参数才下沉；避免 category 包吞入通用职责。
- router：构造 category service 时注入 object store 和 logger，注册两个 admin 上传路由。

## 错误语义

以下返回 400 和可读中文消息：文件缺失、格式不支持、SVG 不安全、体积超限、素材 key 无效、外链、越权引用、对象不存在。鉴权仍由 admin middleware 返回 401/403；对象存储不可用或数据库错误返回 500。

上传和保存错误应保留稳定的 service sentinel，handler 使用 `errors.Is` 映射，不匹配错误字符串。

## 测试要求

严格按 TDD，先新增失败测试并确认 RED。

### SVG 校验器

- 最小合法 path SVG、渐变和本地 `use` 通过。
- script、事件属性、style、foreignObject、外链 href、外部 `url()`、DOCTYPE、未知节点失败。
- 超体积、过深、节点/属性过多失败。
- 输出为规范化 SVG，MIME 固定。

### service

- 三个可选字段全空可创建，数据库保存 nil。
- 封面复用文章压缩参数并生成当前用户临时 key。
- 图标上传生成内容哈希临时 key。
- 创建/更新把本人临时素材转为分类正式 key。
- 拒绝其他用户临时 key、其他分类 key、外链和不存在对象。
- 未传、清空、保持、替换四种更新语义。
- DB/复制失败的回滚与清理顺序；成功后清理 temp/旧素材。
- 响应 key 解析成 URL。

### handler/router/repository

- handler 从 claims 传递 user ID，multipart 缺文件和超限响应正确。
- 两个 admin 路由已注册且受鉴权保护。
- repository prepare callback 在事务中取得 ID，并在错误时回滚。
- 更新/删除不误删文章或其他分类资源。

建议验证命令：

```bash
go test ./internal/service/category ./internal/handler/category ./internal/repository/category
go test ./pkg/...
go test ./...
make swag
```

最后检查 `git diff --check`，并确认生成的 Swagger 只反映新契约，不包含无关重写。

## 范围外

- 不允许任意外链分类素材。
- 不给标签模块顺带增加上传接口。
- 不建设通用素材库、裁剪器或后台定时清理任务。
- 不改变文章封面现有压缩结果；只复用其参数与处理路径。

## 风险与决策

- SVG 是主动内容，必须坚持服务端白名单和重新编码；仅检查 MIME/扩展名不可接受。
- 数据库事务无法覆盖对象存储，必须显式补偿清理，并确保先提交新引用、后删旧对象。
- 若现有历史数据含外链，只允许原值保持或人工迁移，不允许借编辑接口新增外链。
- 临时对象未保存时可能残留，本次只做内容哈希幂等与保存后清理；定时清理属于后续运维任务。

## 可直接交给后端 Agent 的提示词

```text
你负责在 /Users/vpt/Documents/Codes/blog/blog-backend 独立完成“分类可选描述、SVG 图标上传、封面上传及资源持久化”后端实现。

开始前：
1. 阅读仓库根 AGENTS.md。
2. 完整阅读 docs/superpowers/specs/2026-07-05-category-assets-backend-design.md。
3. 按任务触发并遵守 go-layering、http-api、go-readability、go-testing；修 bug 用 systematic-debugging，实施用 test-driven-development，完成前用 verification-before-completion。
4. 先检查 git status，保留并避开用户已有改动。

实施要求：
- 只改 blog-backend，不改 frontend。
- 先写会因旧行为失败的测试并实际运行确认 RED，再写最小实现至 GREEN。
- 分类 icon、description、cover_img_url 创建和更新均为可选；名称和 seq 继续必填。
- 新增 admin 分类 SVG 图标和位图封面 multipart 上传接口，claims user ID 必须贯穿临时 key 与保存校验。
- 封面严格复用文章封面的读取、校验、压缩和存储参数，避免复制漂移。
- SVG 必须按文档做 XML 节点/属性白名单、资源上限和重新编码；禁止仅凭扩展名/MIME 放行。
- 临时资源保存时转到 categories/{id}/...，只接受本人临时 key或本分类正式资源；拒绝外链和越权 key。
- 创建使用事务内 prepare 流程取得 ID；正确处理替换、清空、回滚、temp/旧素材清理和删除分类后的清理。
- 数据库存 key，DTO 响应解析为 URL；同步 Swagger 并保持 handler/service/repository 分层。
- 不顺手实现标签素材上传或通用素材库。

验收：
- 空描述/图标/封面可正常创建。
- 合法 SVG、封面上传和创建/编辑/清空全链路通过；恶意 SVG、外链、越权 key 明确拒绝。
- 相关分层测试、go test ./...、make swag、git diff --check 通过。
- 最终只简要报告改动、验证和残余风险，不提交代码，除非我另行要求。
```
