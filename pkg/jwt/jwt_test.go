package jwt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/jwt"
)

func newTestManager() *jwt.Manager {
	return jwt.NewManager("test-secret", 2, 168)
}

func TestGenerateAccess_TokenType(t *testing.T) {
	m := newTestManager()
	token, err := m.GenerateAccess(1, "alice", []string{"ROLE_NORMAL"})
	require.NoError(t, err)

	claims, err := m.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, "access", claims.TokenType)
	assert.Equal(t, int64(1), claims.UserId)
	assert.Equal(t, "alice", claims.Username)
}

func TestGenerateRefresh_TokenType(t *testing.T) {
	m := newTestManager()
	token, err := m.GenerateRefresh(1, "alice", []string{"ROLE_NORMAL"})
	require.NoError(t, err)

	claims, err := m.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, "refresh", claims.TokenType)
}

func TestParse_InvalidToken(t *testing.T) {
	m := newTestManager()
	_, err := m.Parse("not.a.token")
	assert.ErrorIs(t, err, jwt.ErrTokenInvalid)
}
