package ruleindex_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

func TestCodecRoundTripPreservesMatchResult(t *testing.T) {
	before := buildSnapshot(t, []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "风险", Risk: "medium", Effect: "review"},
		{ID: 2, Type: "regexp", Pattern: `VX\d+`, Risk: "high", Effect: "review"},
		{ID: 3, Type: "composite", Pattern: "扫码&&入群", Risk: "high", Effect: "review"},
	}, defaultLimits())
	var encoded bytes.Buffer

	checksum, err := before.WriteEncodedTo(&encoded)
	require.NoError(t, err)
	after, loadedChecksum, err := ruleindex.ReadFrom(bytes.NewReader(encoded.Bytes()), defaultLimits())
	require.NoError(t, err)

	assert.Equal(t, checksum, loadedChecksum)
	assert.Equal(t, int64(encoded.Len()), before.EncodedSize())
	assert.Equal(t, before.Version(), after.Version())
	assert.Equal(t, before.Match(textnorm.Normalize("风险 vx123 请扫码后入群")), after.Match(textnorm.Normalize("风险 vx123 请扫码后入群")))
}

func TestCodecWritesDeterministicBytes(t *testing.T) {
	snapshot := buildSnapshot(t, nestedRules(30), defaultLimits())
	var first bytes.Buffer
	var second bytes.Buffer

	firstHash, err := snapshot.WriteEncodedTo(&first)
	require.NoError(t, err)
	secondHash, err := snapshot.WriteEncodedTo(&second)
	require.NoError(t, err)

	assert.Equal(t, firstHash, secondHash)
	assert.Equal(t, first.Bytes(), second.Bytes())
}

func TestReadRejectsDeclaredArrayBeforeAllocating(t *testing.T) {
	raw := encodedHeaderWithStateCount(math.MaxUint32)

	_, _, err := ruleindex.ReadFrom(bytes.NewReader(raw), defaultLimits())

	assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)
}

func TestReadRejectsChecksumMismatch(t *testing.T) {
	snapshot := buildSnapshot(t, []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "风险", Risk: "medium", Effect: "review"},
	}, defaultLimits())
	var encoded bytes.Buffer
	_, err := snapshot.WriteEncodedTo(&encoded)
	require.NoError(t, err)
	raw := encoded.Bytes()
	raw[len(raw)-1] ^= 0xff

	_, _, err = ruleindex.ReadFrom(bytes.NewReader(raw), defaultLimits())

	assert.ErrorIs(t, err, ruleindex.ErrIndexCorrupt)
}

func TestReadRejectsDeclaredMemoryBeforeAllocating(t *testing.T) {
	snapshot := buildSnapshot(t, []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "风险", Risk: "medium", Effect: "review"},
	}, defaultLimits())
	var encoded bytes.Buffer
	_, err := snapshot.WriteEncodedTo(&encoded)
	require.NoError(t, err)
	limits := defaultLimits()
	limits.MaxIndexMemoryBytes = 1

	_, _, err = ruleindex.ReadFrom(bytes.NewReader(encoded.Bytes()), limits)

	assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)
}

func TestBuildRejectsRetainedAndPeakBudgets(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "风险", Risk: "medium", Effect: "review"},
	}

	retainedLimit := defaultLimits()
	retainedLimit.MaxIndexMemoryBytes = 1
	_, _, err := ruleindex.Build(t.Context(), 1, sliceSource(rules), retainedLimit)
	assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)

	peakLimit := defaultLimits()
	peakLimit.MaxIndexMemoryBytes = 1 << 30
	peakLimit.MaxBuildPeakMemoryBytes = 1
	_, _, err = ruleindex.Build(t.Context(), 1, sliceSource(rules), peakLimit)
	assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)
}

func TestBuildReportsConservativeMemoryStats(t *testing.T) {
	snapshot := buildSnapshot(t, nestedRules(30), defaultLimits())
	stats := snapshot.Stats()

	assert.Positive(t, stats.IndexBytes)
	assert.Greater(t, stats.BuildPeakBytes, stats.IndexBytes)
}

func encodedHeaderWithStateCount(stateCount uint32) []byte {
	var raw bytes.Buffer
	raw.WriteString("BMR1")
	requireNoError(binary.Write(&raw, binary.LittleEndian, uint32(1)))
	requireNoError(binary.Write(&raw, binary.LittleEndian, uint64(1)))
	for _, value := range []uint32{128, stateCount, 0, 0, 0, 0, 0, 0, 0} {
		requireNoError(binary.Write(&raw, binary.LittleEndian, value))
	}
	return raw.Bytes()
}

func requireNoError(err error) {
	if err != nil {
		panic(err)
	}
}
