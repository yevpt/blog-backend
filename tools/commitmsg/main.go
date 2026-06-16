// Command commitmsg 校验 git commit message 格式（与工具/AI 无关，git 层强制）。
// 规范：<type>(<scope>): <中文主题>，详见 .agents/skills/git-commit/SKILL.md。
// 由 .githooks/commit-msg 调用，参数为 commit message 文件路径。
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxSubject = 50

var allowedTypes = map[string]bool{
	"feat": true, "fix": true, "refactor": true, "test": true, "chore": true,
	"perf": true, "docs": true, "ci": true, "style": true, "build": true,
}

var (
	headerRe   = regexp.MustCompile(`^([a-z]+)(?:\(([a-z0-9-]+)\))?: (.+)$`)
	hanRe      = regexp.MustCompile(`\p{Han}`)
	aiSignRe   = regexp.MustCompile(`(?i)generated (with|by)\b|🤖|noreply@\S+|by \[?\w+`)
	coAuthorRe = regexp.MustCompile(`(?im)^Co-authored-by:`)
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "[commit-msg] 缺少 message 文件参数")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[commit-msg] 读取失败: %v\n", err)
		os.Exit(1)
	}
	raw := string(data)

	// 取首个非空、非注释行作为主题
	subject := ""
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		l := sc.Text()
		if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "#") {
			subject = l
			break
		}
	}

	// 放行 git 自动生成的 merge / revert
	if subject == "" || strings.HasPrefix(subject, "Merge ") || strings.HasPrefix(subject, "Revert ") {
		os.Exit(0)
	}

	var errs []string

	if m := headerRe.FindStringSubmatch(subject); m == nil {
		errs = append(errs, "格式必须为 `<type>(<scope>): <中文主题>`（scope 可选，冒号后留一个空格）")
	} else {
		typ, sub := m[1], m[3]
		if !allowedTypes[typ] {
			errs = append(errs, fmt.Sprintf("type 非法：「%s」。仅允许 feat/fix/refactor/test/chore/perf/docs/ci/style/build", typ))
		}
		if utf8.RuneCountInString(sub) > maxSubject {
			errs = append(errs, fmt.Sprintf("主题超长：%d 字，需 ≤ %d 字", utf8.RuneCountInString(sub), maxSubject))
		}
		if !hanRe.MatchString(sub) {
			errs = append(errs, "主题需使用中文描述")
		}
		if strings.HasSuffix(sub, "。") || strings.HasSuffix(sub, ".") {
			errs = append(errs, "主题结尾不要加句号")
		}
	}

	if strings.Contains(strings.ToLower(raw), "breaking change") && !strings.Contains(raw, "BREAKING CHANGE:") {
		errs = append(errs, "破坏性变更标记须为 `BREAKING CHANGE: <描述>`（全大写 + 冒号）")
	}
	if coAuthorRe.MatchString(raw) {
		errs = append(errs, "禁止添加 `Co-authored-by:` 署名")
	}
	if aiSignRe.MatchString(raw) {
		errs = append(errs, "禁止添加 AI 生成标记 / 工具署名（如 Generated with、🤖、Claude Code 等）")
	}

	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "\n✗ Commit message 不符合规范：\n")
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "  - "+e)
		}
		fmt.Fprintln(os.Stderr, "\n  示例：feat(auth): 新增邮箱验证码登录")
		fmt.Fprintln(os.Stderr, "  规范详见 .agents/skills/git-commit/SKILL.md\n")
		os.Exit(1)
	}
}
