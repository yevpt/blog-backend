package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSetupOptions_DefaultsAdminAccount(t *testing.T) {
	opts, err := parseSetupOptions([]string{})

	require.NoError(t, err)
	assert.Equal(t, "admin", opts.Seed.AdminUsername)
	assert.Equal(t, "admin", opts.Seed.AdminPassword)
}

func TestParseSetupOptions_FlagOverridesEnv(t *testing.T) {
	t.Setenv("BLOG_DBSETUP_ADMIN_PASSWORD", "env-password")

	opts, err := parseSetupOptions([]string{"--admin-password", "flag-password", "--admin-username", "root"})

	require.NoError(t, err)
	assert.Equal(t, "flag-password", opts.Seed.AdminPassword)
	assert.Equal(t, "root", opts.Seed.AdminUsername)
}

func TestParseSetupOptions_ReadsAdminPasswordFromEnv(t *testing.T) {
	t.Setenv("BLOG_DBSETUP_ADMIN_PASSWORD", "secret-password")

	opts, err := parseSetupOptions([]string{})

	require.NoError(t, err)
	assert.Equal(t, "secret-password", opts.Seed.AdminPassword)
}
