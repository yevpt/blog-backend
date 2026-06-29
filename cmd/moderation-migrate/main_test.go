package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOptionsSupportsResumeAndVerify(t *testing.T) {
	got, err := parseOptions([]string{
		"--batch-size", "50", "--after-type", "article_comment", "--after-id", "99", "--verify-only",
	}, 200)

	require.NoError(t, err)
	assert.Equal(t, 50, got.BatchSize)
	assert.Equal(t, "article_comment", got.AfterType)
	assert.Equal(t, uint64(99), got.AfterID)
	assert.True(t, got.VerifyOnly)
}

func TestParseOptionsRejectsUnboundedBatch(t *testing.T) {
	_, err := parseOptions([]string{"--batch-size", "5001"}, 200)
	require.Error(t, err)
}
