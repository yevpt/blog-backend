## Git Commit 规范

commit message 由 `commit-msg` 钩子（`scripts/validate-commit-msg.cjs`）强制校验，不合规会被拒。请第一次就写对。

格式：

```text
<type>(<scope>): <中文主题>
```

可选正文与 footer。

---

### 1. type 规范

`type` 必填，必须小写，只能使用以下值：

* `feat`：新功能
* `fix`：修 bug
* `refactor`：重构
* `perf`：性能优化
* `test`：测试
* `docs`：文档
* `style`：纯格式调整，不改变逻辑
* `build`：构建相关
* `chore`：脚手架、依赖、杂项维护
* `ci`：CI/CD 相关

---

### 2. scope 规范

`scope` 可选，用于说明影响范围。

要求：

* 使用英文小写技术词
* 可以包含数字和连字符
* 不要使用中文
* 不要使用大写字母

示例：

```text
auth
comment-input
comment-replies
api
ci
```

---

### 3. subject 主题规范

主题指冒号后面的内容。

要求：

* 冒号后必须留一个空格
* 必须使用中文
* 必须以动词开头
* 长度不超过 50 字
* 结尾不要加句号

推荐动词：

```text
新增、添加、实现、支持、接入、修复、解决、优化、提升、重构、调整、更新、升级、迁移、移除、删除、清理、简化
```

---

### 4. 正文规范

正文可选。

如果需要正文，使用中文 bullet points 描述具体变更，技术词可以保留英文。

示例：

```text
- 收敛登录接口的请求参数类型
- 补充 token 过期时的错误处理
- 调整 auth client 的导出方式
```

---

### 5. 破坏性变更

如果包含破坏性变更，在 footer 中添加：

```text
BREAKING CHANGE: <描述>
```

要求：

* `BREAKING CHANGE` 必须全大写
* 后面必须使用英文冒号
* 冒号后留一个空格

---

### 6. 禁止内容

commit message 中禁止出现任何 AI 署名或生成标记，包括但不限于：

```text
Co-authored-by:
Generated with
🤖
Claude Code
```

也不要添加任何类似“由 AI 生成”“由 Claude 生成”的内容。

---

### 7. 正确示例

```text
feat(auth): 新增邮箱验证码登录
```

```text
fix(comment-replies): 修复折叠态缺少上边距
```

```text
refactor(api): 收敛文章请求到类型化 client
```

```text
ci(deploy): 优化后端镜像构建流程

- 拆分测试和镜像构建任务
- 复用 GitHub SHA 作为镜像 tag
- 补充 workflow_dispatch 手动触发入口
```

```text
refactor(auth): 调整登录态存储结构

- 统一 token 和用户信息的读取入口
- 移除旧版 localStorage 字段
- 更新登录态恢复逻辑

BREAKING CHANGE: 旧版 auth_user 字段不再兼容
```

---

### 8. 常见错误

错误：

```text
更新代码
```

原因：缺少 `type`。

错误：

```text
fix: lint errors
```

原因：主题不是中文。

错误：

```text
fix:修复登录失败
```

原因：冒号后缺少空格。

错误：

```text
fix(Auth): 修复登录失败
```

原因：scope 含大写字母。

错误：

```text
fix(auth): 修复登录失败。
```

原因：主题结尾带句号。

错误：

```text
fix(auth): 修复登录失败并且调整用户信息缓存逻辑以及重构认证请求封装和错误处理流程
```

原因：主题过长，超过 50 字。

---

### 9. 生成 commit message 时的要求

生成 commit message 时：

1. 先判断变更类型，选择合适的 `type`
2. 根据影响范围选择是否添加 `scope`
3. 主题必须用中文动词开头
4. 如果变更较多，正文使用中文 bullet points
5. 不要添加 AI 署名、生成标记或无关说明
6. 确保最终结果能通过 `scripts/validate-commit-msg.cjs` 校验
