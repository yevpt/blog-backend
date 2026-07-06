package svgfile_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/svgfile"
)

const minimalSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <path d="M0 0h24v24H0z"/>
</svg>`

const gradientSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <defs>
    <linearGradient id="g1" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#fff"/>
    </linearGradient>
  </defs>
  <rect fill="url(#g1)" width="100" height="100"/>
</svg>`

const localUseSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <defs>
    <path id="p1" d="M0 0h24v24H0z"/>
  </defs>
  <use href="#p1"/>
</svg>`

func TestSVGFile_MinimalPathSVG_Passes(t *testing.T) {
	result, err := svgfile.Validate([]byte(minimalSVG))
	require.NoError(t, err)
	assert.Equal(t, "image/svg+xml", result.ContentType)
	assert.NotEmpty(t, result.Data)
	// 输出必须是合法 SVG（重新编码）
	assert.Contains(t, string(result.Data), "<svg")
}

func TestSVGFile_GradientSVG_Passes(t *testing.T) {
	_, err := svgfile.Validate([]byte(gradientSVG))
	require.NoError(t, err)
}

func TestSVGFile_LocalUseSVG_Passes(t *testing.T) {
	_, err := svgfile.Validate([]byte(localUseSVG))
	require.NoError(t, err)
}

func TestSVGFile_ScriptElement_Fails(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_EventAttribute_Fails(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><path onclick="alert(1)" d="M0 0"/></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_StyleAttribute_Fails(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:red"/></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_ForeignObject_Fails(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><div>hi</div></foreignObject></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_ExternalHref_Fails(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><use href="http://evil.com/evil.svg#x"/></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_ExternalURLInFill_Fails(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><rect fill="url(http://evil.com/img.png)"/></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_DOCTYPE_Fails(t *testing.T) {
	svg := `<?xml version="1.0"?><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" ""><svg xmlns="http://www.w3.org/2000/svg"></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_UnknownElement_Fails(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><animate attributeName="x"/></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_TooLarge_Fails(t *testing.T) {
	// 超过 256KB
	bigContent := strings.Repeat("A", 257*1024)
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><title>` + bigContent + `</title></svg>`
	_, err := svgfile.Validate([]byte(svg))
	require.Error(t, err)
}

func TestSVGFile_NonSVGRoot_Fails(t *testing.T) {
	data := []byte(`<div>not an svg</div>`)
	_, err := svgfile.Validate(data)
	require.Error(t, err)
}

func TestSVGFile_EmptyInput_Fails(t *testing.T) {
	_, err := svgfile.Validate([]byte{})
	require.Error(t, err)
}

func TestSVGFile_TooDeep_Fails(t *testing.T) {
	// 生成深度超过限制的 SVG
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg">`)
	for i := 0; i < 60; i++ {
		b.WriteString(`<g>`)
	}
	for i := 0; i < 60; i++ {
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	_, err := svgfile.Validate([]byte(b.String()))
	require.Error(t, err)
}

func TestSVGFile_TooManyElements_Fails(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg">`)
	for i := 0; i < 1100; i++ {
		b.WriteString(`<path d="M0 0"/>`)
	}
	b.WriteString(`</svg>`)
	_, err := svgfile.Validate([]byte(b.String()))
	require.Error(t, err)
}

func TestSVGFile_SHA256Key_Generated(t *testing.T) {
	result, err := svgfile.Validate([]byte(minimalSVG))
	require.NoError(t, err)
	assert.Len(t, result.SHA256, 64, "SHA256 应为 64 个十六进制字符")
}
