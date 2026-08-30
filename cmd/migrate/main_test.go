package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMigrateOptions_Status(t *testing.T) {
	opts, err := parseMigrateOptions([]string{"status"})

	require.NoError(t, err)
	assert.Equal(t, "status", opts.Action)
}

func TestParseMigrateOptions_AdoptRequiresThrough(t *testing.T) {
	_, err := parseMigrateOptions([]string{"adopt"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--through")
}

func TestParseMigrateOptions_AdoptReadsThrough(t *testing.T) {
	opts, err := parseMigrateOptions([]string{
		"adopt", "--through", "20260704_admin_operation_log.sql",
	})

	require.NoError(t, err)
	assert.Equal(t, "adopt", opts.Action)
	assert.Equal(t, "20260704_admin_operation_log.sql", opts.Through)
}

func TestParseMigrateOptions_RejectsUnknownCommand(t *testing.T) {
	_, err := parseMigrateOptions([]string{"down"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "未知命令")
}
