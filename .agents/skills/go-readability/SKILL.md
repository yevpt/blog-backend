---
name: "go-readability"
description: "Use when organizing code, refactoring, splitting files, or writing comments in this Go backend — covers entry-file facades, splitting by responsibility, short functions, semantic naming, and the Chinese comment conventions. Trigger whenever a file grows large, a function does too many things, you create a new module, or you add comments."
license: "MIT"
metadata:
  scope: "project"
---

# 代码组织与可读性

代码先为人阅读：顺着路径能进入、结构清爽克制。

## 组织

- 每个包有清晰入口文件（`storage.go`/`client.go`/`service.go`/`router.go`），只放对外类型、构造函数、公开方法。
- 入口是门面：mock 接口、内部状态、第三方适配、算法细节别放入口。
- 内部按职责拆文件（初始化、签名、路径、配置转换、数据查询、业务策略、测试辅助），一个文件一种职责，禁大杂烩。
- 阅读路径：先看入口懂「做什么/怎么用」，再按需进细节。

## 拆分信号（避免现存的过载文件）

- **一个文件混 ≥2 类职责就拆**：如 user 文件同时塞 CRUD + 账号安全（改密/改邮箱）+ 资料编辑 → 按职责分文件。
- 函数同时做校验、初始化、分支策略、第三方调用、结果转换时，拆成有名字的小函数。
- 命名体现业务语义与阅读顺序，内部函数也要从名字看出职责。
- 扁平包缺门面文件就补一个。

## 注释（中文，技术术语除外）

- 公开方法/函数：职责 + 关键约束。
- 公开类型/常量/包级变量：用途。
- 结构体字段（尤其 DTO、配置）：含义。
- 函数体每个逻辑步骤一行注释：做什么、为什么。

用户说「看得头大」时，先调组织和阅读路径，而非只补注释。新增独立模块/工具时优先补 `README.md` 或包级说明。
