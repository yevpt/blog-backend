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
	token, err := m.GenerateAccess(1)
	require.NoError(t, err)

	claims, err := m.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, "access", claims.TokenType)
	assert.Equal(t, int64(1), claims.UserId)
}

func TestGenerateRefresh_TokenType(t *testing.T) {
	m := newTestManager()
	token, err := m.GenerateRefresh(1)
	require.NoError(t, err)

	claims, err := m.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, "refresh", claims.TokenType)
	assert.Equal(t, int64(1), claims.UserId)
}

func TestParse_InvalidToken(t *testing.T) {
	m := newTestManager()
	_, err := m.Parse("not.a.token")
	assert.ErrorIs(t, err, jwt.ErrTokenInvalid)
}

func TestParse_ExpiredToken(t *testing.T) {
	// expireHours=-1 让 token 立即过期
	m := jwt.NewManager("test-secret", -1, 168)
	token, err := m.GenerateAccess(1)
	require.NoError(t, err)

	_, parseErr := m.Parse(token)
	assert.ErrorIs(t, parseErr, jwt.ErrTokenExpired)
}

func TestParse_WrongSecret(t *testing.T) {
	m1 := jwt.NewManager("secret-A", 2, 168)
	m2 := jwt.NewManager("secret-B", 2, 168)

	token, err := m1.GenerateAccess(1)
	require.NoError(t, err)

	_, parseErr := m2.Parse(token)
	assert.ErrorIs(t, parseErr, jwt.ErrTokenInvalid)
}
