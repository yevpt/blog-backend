// Package svgfile 实现 SVG 图标的静态安全校验与规范化。
//
// 采用 XML token 白名单：只允许明确列出的节点和属性；
// 校验通过后从解析结果重新编码，不原样信任上传字节。
package svgfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// MaxSVGBytes SVG 图标上限 256 KB。
const MaxSVGBytes = 256 * 1024

// 解析资源限制。
const (
	maxDepth      = 50
	maxElements   = 1000
	maxAttributes = 50
	maxTextBytes  = 4096
)

var (
	// ErrSVGTooLarge SVG 文件超出大小限制。
	ErrSVGTooLarge = errors.New("SVG 图标不能超过 256KB")
	// ErrSVGInvalid SVG 内容不合法或包含危险元素。
	ErrSVGInvalid = errors.New("SVG 内容不合法或包含危险元素")
	// ErrSVGEmpty 上传内容为空。
	ErrSVGEmpty = errors.New("SVG 内容不能为空")
)

// Result SVG 校验结果。
type Result struct {
	Data        []byte // 规范化后的 SVG 字节
	ContentType string // 固定为 image/svg+xml
	SHA256      string // 内容哈希（十六进制）
}

// allowedElements 允许使用的 SVG 节点白名单（全小写，匹配时元素名先 ToLower）。
var allowedElements = map[string]struct{}{
	"svg": {}, "g": {}, "path": {}, "rect": {}, "circle": {},
	"ellipse": {}, "line": {}, "polyline": {}, "polygon": {},
	"title": {}, "desc": {}, "defs": {}, "clippath": {}, "mask": {},
	"lineargradient": {}, "radialgradient": {}, "stop": {}, "use": {},
}

// allowedAttributes 允许使用的属性白名单（小写）。
var allowedAttributes = map[string]struct{}{
	// 几何
	"d": {}, "cx": {}, "cy": {}, "r": {}, "rx": {}, "ry": {},
	"x": {}, "y": {}, "x1": {}, "y1": {}, "x2": {}, "y2": {},
	"width": {}, "height": {}, "points": {},
	// 视图盒和命名空间
	"viewbox": {}, "xmlns": {}, "xmlns:xlink": {}, "version": {},
	// 颜色与描边
	"fill": {}, "stroke": {}, "stroke-width": {}, "stroke-linecap": {},
	"stroke-linejoin": {}, "stroke-miterlimit": {}, "stroke-dasharray": {},
	"stroke-dashoffset": {}, "fill-opacity": {}, "stroke-opacity": {},
	"opacity": {},
	// 变换
	"transform": {}, "gradienttransform": {},
	// 渐变
	"gradientunits": {}, "spreadmethod": {},
	"offset": {}, "stop-color": {}, "stop-opacity": {},
	// 裁剪和遮罩
	"clip-path": {}, "mask": {}, "clip-rule": {}, "fill-rule": {},
	"clippatternunits": {}, "maskunits": {}, "maskcontentunits": {},
	// 引用
	"id": {}, "href": {}, "xlink:href": {},
	// 其他安全属性
	"display": {}, "visibility": {}, "overflow": {},
	"preserveaspectratio": {},
	"color": {}, "color-interpolation": {}, "color-rendering": {},
	"shape-rendering": {}, "text-rendering": {},
	"markerunits": {}, "marker-start": {}, "marker-mid": {}, "marker-end": {},
}

// Validate 校验并规范化 SVG 图标；返回重新编码的结果。
// 校验失败返回哨兵错误，调用方通过 errors.Is 判断。
func Validate(data []byte) (Result, error) {
	if len(data) == 0 {
		return Result{}, ErrSVGEmpty
	}
	if len(data) > MaxSVGBytes {
		return Result{}, ErrSVGTooLarge
	}

	clean, err := sanitize(data)
	if err != nil {
		return Result{}, err
	}

	sum := sha256.Sum256(clean)
	return Result{
		Data:        clean,
		ContentType: "image/svg+xml",
		SHA256:      hex.EncodeToString(sum[:]),
	}, nil
}

// svgNode 解析树节点，用于重新编码。
type svgNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr
	Content []svgChild
}

type svgChild struct {
	node *svgNode
	text string
}

// sanitize 解析并白名单校验 SVG，返回规范化字节。
func sanitize(data []byte) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	// 禁止 DOCTYPE/ENTITY
	dec.Strict = true
	dec.Entity = nil

	root, err := parseNode(dec, 0, new(int))
	if err != nil {
		return nil, wrapInvalid(err)
	}

	// 根节点必须是 svg
	if strings.ToLower(root.XMLName.Local) != "svg" {
		return nil, fmt.Errorf("%w：根节点必须是 svg", ErrSVGInvalid)
	}

	var buf bytes.Buffer
	if err := encodeNode(&buf, root); err != nil {
		return nil, wrapInvalid(err)
	}
	return buf.Bytes(), nil
}

// parseNode 递归解析 XML token，并执行白名单检查。
func parseNode(dec *xml.Decoder, depth int, totalElements *int) (*svgNode, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("%w：嵌套深度超过限制 %d", ErrSVGInvalid, maxDepth)
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("%w：意外结束", ErrSVGInvalid)
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return parseStartElement(dec, t, depth, totalElements)
		case xml.ProcInst:
			// 允许 XML 声明（version/encoding），禁止其他处理指令
			if strings.ToLower(t.Target) != "xml" {
				return nil, fmt.Errorf("%w：禁止处理指令 %s", ErrSVGInvalid, t.Target)
			}
		case xml.Directive:
			// DOCTYPE/ENTITY
			return nil, fmt.Errorf("%w：禁止 DOCTYPE/ENTITY", ErrSVGInvalid)
		case xml.EndElement, xml.CharData, xml.Comment:
			// 顶层这些 token 忽略（注释安全，字符数据忽略）
		}
	}
}

// parseStartElement 解析开始元素及其子元素。
func parseStartElement(dec *xml.Decoder, start xml.StartElement, depth int, totalElements *int) (*svgNode, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("%w：嵌套深度超过限制 %d", ErrSVGInvalid, maxDepth)
	}
	*totalElements++
	if *totalElements > maxElements {
		return nil, fmt.Errorf("%w：节点数超过限制 %d", ErrSVGInvalid, maxElements)
	}

	elemName := strings.ToLower(start.Name.Local)
	if _, ok := allowedElements[elemName]; !ok {
		return nil, fmt.Errorf("%w：不允许的节点 <%s>", ErrSVGInvalid, elemName)
	}

	// 校验属性
	if len(start.Attr) > maxAttributes {
		return nil, fmt.Errorf("%w：属性数超过限制 %d", ErrSVGInvalid, maxAttributes)
	}
	cleanAttrs, err := validateAttributes(start.Attr)
	if err != nil {
		return nil, err
	}

	node := &svgNode{
		XMLName: start.Name,
		Attrs:   cleanAttrs,
	}

	// 解析子节点
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("%w：意外结束", ErrSVGInvalid)
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, childErr := parseStartElement(dec, t, depth+1, totalElements)
			if childErr != nil {
				return nil, childErr
			}
			node.Content = append(node.Content, svgChild{node: child})
		case xml.EndElement:
			return node, nil
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if len(text) > maxTextBytes {
				return nil, fmt.Errorf("%w：文本内容超过限制 %d 字节", ErrSVGInvalid, maxTextBytes)
			}
			if text != "" {
				node.Content = append(node.Content, svgChild{text: text})
			}
		case xml.Comment:
			// 注释：忽略（不输出）
		case xml.ProcInst, xml.Directive:
			return nil, fmt.Errorf("%w：禁止处理指令或 DOCTYPE", ErrSVGInvalid)
		}
	}
}

// validateAttributes 检查属性白名单并返回清洗后的属性列表。
func validateAttributes(attrs []xml.Attr) ([]xml.Attr, error) {
	result := make([]xml.Attr, 0, len(attrs))
	for _, attr := range attrs {
		attrName := strings.ToLower(attr.Name.Local)
		// 拒绝事件属性 on*
		if strings.HasPrefix(attrName, "on") {
			return nil, fmt.Errorf("%w：禁止事件属性 %s", ErrSVGInvalid, attr.Name.Local)
		}
		// 拒绝 style 属性
		if attrName == "style" {
			return nil, fmt.Errorf("%w：禁止 style 属性", ErrSVGInvalid)
		}
		// 拒绝 data-* 以外不在白名单的属性（data-* 也不允许）
		qualified := strings.ToLower(attr.Name.Space+":"+attr.Name.Local)
		if attr.Name.Space == "" {
			qualified = attrName
		}
		if _, ok := allowedAttributes[qualified]; !ok {
			if _, ok2 := allowedAttributes[attrName]; !ok2 {
				return nil, fmt.Errorf("%w：不允许的属性 %s", ErrSVGInvalid, attr.Name.Local)
			}
		}
		// 对 href / xlink:href 只允许 # 开头（本地引用）
		if attrName == "href" || qualified == "xlink:href" {
			if err := validateRefAttr(attr.Value, attr.Name.Local); err != nil {
				return nil, err
			}
		}
		// 对 fill/stroke/clip-path/mask 中的 url() 检查
		if attrName == "fill" || attrName == "stroke" || attrName == "clip-path" || attrName == "mask" {
			if err := validateURLFuncAttr(attr.Value, attr.Name.Local); err != nil {
				return nil, err
			}
		}
		result = append(result, attr)
	}
	return result, nil
}

// validateRefAttr 确保 href/xlink:href 只允许本地 #id 引用。
func validateRefAttr(value, attrName string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	if !strings.HasPrefix(v, "#") {
		return fmt.Errorf("%w：%s 只允许本地 #id 引用，拒绝 %s", ErrSVGInvalid, attrName, v)
	}
	return nil
}

// validateURLFuncAttr 确保 url(...) 只引用本地 fragment。
func validateURLFuncAttr(value, attrName string) error {
	v := strings.TrimSpace(value)
	if !strings.Contains(strings.ToLower(v), "url(") {
		return nil
	}
	// 提取 url(...) 内的值
	lower := strings.ToLower(v)
	start := strings.Index(lower, "url(")
	if start < 0 {
		return nil
	}
	inner := v[start+4:]
	end := strings.Index(inner, ")")
	if end < 0 {
		return fmt.Errorf("%w：%s 的 url() 格式不合法", ErrSVGInvalid, attrName)
	}
	ref := strings.Trim(inner[:end], `"' `)
	if !strings.HasPrefix(ref, "#") {
		return fmt.Errorf("%w：%s 的 url() 只允许本地 #id 引用，拒绝 %s", ErrSVGInvalid, attrName, ref)
	}
	return nil
}

// encodeNode 将解析树规范化输出为 SVG 字节。
func encodeNode(w io.Writer, node *svgNode) error {
	enc := xml.NewEncoder(w)
	if err := writeNode(enc, node); err != nil {
		return err
	}
	return enc.Flush()
}

func writeNode(enc *xml.Encoder, node *svgNode) error {
	start := xml.StartElement{Name: node.XMLName, Attr: node.Attrs}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	for _, child := range node.Content {
		if child.node != nil {
			if err := writeNode(enc, child.node); err != nil {
				return err
			}
		} else if child.text != "" {
			if err := enc.EncodeToken(xml.CharData(child.text)); err != nil {
				return err
			}
		}
	}
	return enc.EncodeToken(xml.EndElement{Name: node.XMLName})
}

func wrapInvalid(err error) error {
	if errors.Is(err, ErrSVGInvalid) || errors.Is(err, ErrSVGTooLarge) || errors.Is(err, ErrSVGEmpty) {
		return err
	}
	return fmt.Errorf("%w：%v", ErrSVGInvalid, err)
}
