package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPurposeCopy_ReturnsDistinctTextPerScenario(t *testing.T) {
	cases := []struct {
		purpose      Purpose
		wantTitle    string
		wantContains string
	}{
		{PurposeRegister, "注册验证码", "你正在注册"},
		{PurposePasswordReset, "重置密码验证码", "你正在重置"},
		{PurposeEmailBind, "绑定邮箱验证码", "你正在绑定/更换"},
	}
	for _, c := range cases {
		title, desc := purposeCopy(c.purpose, "YEVPT")
		assert.Equal(t, c.wantTitle, title)
		assert.Contains(t, desc, c.wantContains)
		assert.Contains(t, desc, "YEVPT")
	}
}

func TestPurposeCopy_UnknownPurposeFallsBackToRegisterCopy(t *testing.T) {
	title, _ := purposeCopy(Purpose("unknown"), "YEVPT")
	assert.Equal(t, "注册验证码", title)
}
