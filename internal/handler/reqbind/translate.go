package reqbind

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var (
	validatorLabelsOnce sync.Once
	fieldLabels         = map[string]string{
		"article_ids":     "文章 ID 列表",
		"category_id":     "分类 ID",
		"category_ids":    "分类 ID 列表",
		"captcha_token":   "验证码票据",
		"challenge_id":    "挑战 ID",
		"code":            "验证码",
		"comment_status":  "评论状态",
		"content":         "内容",
		"cover_img_url":   "封面图",
		"description":     "描述",
		"email":           "邮箱",
		"icon":            "图标",
		"identifier":      "账号",
		"name":            "名称",
		"owner_user_id":   "留言板主人用户 ID",
		"page":            "页码",
		"page_size":       "每页数量",
		"parent_reply_id": "父级回复 ID",
		"password":        "密码",
		"refresh_token":   "刷新令牌",
		"role_id":         "角色 ID",
		"seq":             "排序值",
		"status":          "状态",
		"tag_id":          "标签 ID",
		"target_id":       "评论目标 ID",
		"target_type":     "评论目标类型",
		"title":           "标题",
		"url":             "链接地址",
		"user_id":         "用户 ID",
		"x":               "滑块 X 坐标",
		"y":               "滑块 Y 坐标",
	}
)

func ensureValidatorLabels() {
	validatorLabelsOnce.Do(func() {
		engine, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}

		engine.RegisterTagNameFunc(func(field reflect.StructField) string {
			return resolveFieldLabel(field)
		})
	})
}

func translateBindingError(err error) string {
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) && len(validationErrs) > 0 {
		return translateValidationError(validationErrs[0])
	}

	if errors.Is(err, io.EOF) {
		return "请求体不能为空"
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return formatTypeError(typeErr.Field, typeErr.Type)
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return "请求体必须是合法的 JSON"
	}

	var numErr *strconv.NumError
	if errors.As(err, &numErr) {
		return "参数格式错误，请输入正确的数字"
	}

	if strings.Contains(err.Error(), "invalid character") {
		return "请求体必须是合法的 JSON"
	}

	return "参数格式错误，请检查输入内容"
}

func translateValidationError(fieldErr validator.FieldError) string {
	label := fieldErr.Field()
	if label == "" {
		label = "参数"
	}

	switch fieldErr.Tag() {
	case "required":
		return label + "不能为空"
	case "email":
		return label + "格式不正确"
	case "min":
		return translateMinError(label, fieldErr.Kind(), fieldErr.Param())
	case "max":
		return translateMaxError(label, fieldErr.Kind(), fieldErr.Param())
	case "len":
		return translateLenError(label, fieldErr.Kind(), fieldErr.Param())
	case "oneof":
		return label + "只能是 " + strings.Join(strings.Fields(fieldErr.Param()), "、")
	default:
		return label + "不合法"
	}
}

func translateMinError(label string, kind reflect.Kind, param string) string {
	switch kind {
	case reflect.String:
		return label + "长度不能短于 " + param + " 个字符"
	case reflect.Slice, reflect.Array:
		return label + "至少需要 " + param + " 项"
	default:
		return label + "不能小于 " + param
	}
}

func translateMaxError(label string, kind reflect.Kind, param string) string {
	switch kind {
	case reflect.String:
		return label + "长度不能超过 " + param + " 个字符"
	case reflect.Slice, reflect.Array:
		return label + "最多只能有 " + param + " 项"
	default:
		return label + "不能大于 " + param
	}
}

func translateLenError(label string, kind reflect.Kind, param string) string {
	switch kind {
	case reflect.String:
		if strings.Contains(label, "验证码") {
			return label + "长度必须为 " + param + " 位"
		}
		return label + "长度必须为 " + param + " 个字符"
	case reflect.Slice, reflect.Array:
		return label + "数量必须为 " + param + " 项"
	default:
		return label + "长度必须为 " + param
	}
}

func formatTypeError(field string, targetType reflect.Type) string {
	label := lookupFieldLabel(field)
	if label == "" {
		label = "参数"
	}

	return label + "类型错误，应为" + readableType(targetType)
}

func readableType(targetType reflect.Type) string {
	if targetType == nil {
		return "正确类型"
	}

	for targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}

	switch targetType.Kind() {
	case reflect.Bool:
		return "布尔值"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "整数"
	case reflect.Float32, reflect.Float64:
		return "数字"
	case reflect.String:
		return "字符串"
	case reflect.Slice, reflect.Array:
		return "数组"
	case reflect.Map, reflect.Struct:
		return "对象"
	default:
		return targetType.String()
	}
}

func resolveFieldLabel(field reflect.StructField) string {
	for _, rawTag := range []string{field.Tag.Get("json"), field.Tag.Get("form"), field.Tag.Get("uri")} {
		name := extractTagName(rawTag)
		if name == "" {
			continue
		}

		if label, ok := fieldLabels[name]; ok {
			return label
		}

		return name
	}

	if label, ok := fieldLabels[field.Name]; ok {
		return label
	}

	return field.Name
}

func lookupFieldLabel(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}

	parts := strings.Split(field, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		name := extractTagName(parts[i])
		if name == "" {
			continue
		}
		if label, ok := fieldLabels[name]; ok {
			return label
		}
	}

	last := parts[len(parts)-1]
	if label, ok := fieldLabels[last]; ok {
		return label
	}

	return last
}

func extractTagName(raw string) string {
	if raw == "" || raw == "-" {
		return ""
	}

	name := strings.Split(raw, ",")[0]
	if name == "" || name == "-" {
		return ""
	}

	return name
}
