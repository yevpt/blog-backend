package dbschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/roles"
	"golang.org/x/crypto/bcrypt"
)

func TestBuildDefaultSeedData_IncludesAdminUserWithIDOne(t *testing.T) {
	email := "admin@example.com"

	data := buildDefaultSeedData("$2a$12$hash", SeedOptions{
		AdminUsername: "admin",
		AdminEmail:    email,
	})

	require.Len(t, data.Users, 1)
	admin := data.Users[0]
	assert.Equal(t, uint(1), admin.ID)
	assert.Equal(t, "admin", admin.Username)
	assert.Equal(t, "$2a$12$hash", admin.Password)
	assert.True(t, admin.PasswordSet)
	assert.Equal(t, uint8(1), admin.Status)
	require.NotNil(t, admin.Email)
	assert.Equal(t, email, *admin.Email)

	assert.Contains(t, data.Roles, roleSeed{ID: roles.AdminRoleId, Name: roles.AdminRole})
	assert.Contains(t, data.Roles, roleSeed{ID: roles.NormalRoleId, Name: roles.NormalRole})
	assert.Contains(t, data.Roles, roleSeed{ID: roles.VipRoleId, Name: roles.VipRole})
	assert.Contains(t, data.UserRoles, userRoleSeed{UserID: 1, RoleID: roles.AdminRoleId})
	require.Len(t, data.UserMetas, 1)
	assert.Equal(t, uint(1), data.UserMetas[0].UserID)
	require.Len(t, data.UserSettings, 1)
	assert.Equal(t, uint(1), data.UserSettings[0].UserID)
}

func TestHashAdminPassword_UsesBcrypt(t *testing.T) {
	hash, err := hashAdminPassword("change-me-strong")

	require.NoError(t, err)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("change-me-strong")))
}

func TestSeedDefaults_DefaultsAdminPassword(t *testing.T) {
	hash, err := hashAdminPassword(defaultAdminPassword)

	require.NoError(t, err)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("admin")))
}
