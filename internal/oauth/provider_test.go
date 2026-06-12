package oauth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/oauth"
)

func TestParseAction_AcceptsKnownActions(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want oauth.Action
	}{
		{name: "登录", raw: "login", want: oauth.ActionLogin},
		{name: "绑定", raw: "bind", want: oauth.ActionBind},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oauth.ParseAction(tt.raw)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseAction_RejectsUnknownAction(t *testing.T) {
	tests := []string{"", "delete", "LOGIN", " bind "}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			_, err := oauth.ParseAction(raw)

			assert.ErrorIs(t, err, oauth.ErrInvalidAction)
		})
	}
}

func TestProfile_IdentityKey(t *testing.T) {
	profile := oauth.Profile{Source: "github", UUID: "12345"}

	assert.Equal(t, "github:12345", profile.IdentityKey())
}
