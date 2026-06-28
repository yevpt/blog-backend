package dbschema

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
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

func TestModerationModelsRegisterInDependencyOrder(t *testing.T) {
	models := moderationModels()
	want := []reflect.Type{
		reflect.TypeOf(&model.ModerationItem{}),
		reflect.TypeOf(&model.ModerationRevision{}),
		reflect.TypeOf(&model.ModerationRevisionImage{}),
		reflect.TypeOf(&model.ModerationImage{}),
		reflect.TypeOf(&model.ModerationAttempt{}),
		reflect.TypeOf(&model.ModerationRule{}),
		reflect.TypeOf(&model.ModerationActionLog{}),
		reflect.TypeOf(&model.ModerationVisibleImage{}),
		reflect.TypeOf(&model.UserModerationProfile{}),
		reflect.TypeOf(&model.ModerationControl{}),
	}

	got := make([]reflect.Type, 0, len(models))
	for _, value := range models {
		got = append(got, reflect.TypeOf(value))
	}
	assert.Equal(t, want, got)
}

func TestBuildDefaultSeedData_IncludesModerationDefaults(t *testing.T) {
	data := buildDefaultSeedData("$2a$12$hash", SeedOptions{})

	require.Len(t, data.ModerationRules, 2)
	assert.Equal(t, model.ModerationRiskLow, data.ModerationRules[0].RiskLevel)
	assert.True(t, data.ModerationRules[0].Enabled)
	assert.False(t, data.ModerationRules[1].Enabled)
	for _, rule := range data.ModerationRules {
		assert.NotEmpty(t, rule.Name)
		assert.NotZero(t, rule.RulesetVersion)
	}

	require.Len(t, data.ModerationControls, 1)
	control := data.ModerationControls[0]
	assert.Equal(t, uint64(1), control.ID)
	assert.Equal(t, model.ModerationRegistrationOpen, control.RegistrationMode)
	assert.Equal(t, model.ModerationPublishingOpen, control.PublishingMode)
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
