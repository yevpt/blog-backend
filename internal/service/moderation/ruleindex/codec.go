package ruleindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"unicode/utf8"
	"unsafe"

	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

const (
	magic = "BMR1"
	// FormatVersion 是当前二进制索引格式版本。
	FormatVersion        uint32 = 1
	headerSize                  = 52
	stateEncodedSize            = 28
	edgeEncodedSize             = 8
	ruleEncodedSize             = 16
	nonKeywordHeaderSize        = 12
	checksumSize                = sha256.Size
)

type codecHeader struct {
	version         uint64
	maxMatchIDs     uint32
	stateCount      uint32
	edgeCount       uint32
	outputCount     uint32
	ruleCount       uint32
	nonKeywordCount uint32
	keywordCount    uint32
	regexpCount     uint32
	compositeCount  uint32
}

// EncodedSize 返回固定格式索引文件的精确字节数。
func (s *Snapshot) EncodedSize() int64 {
	if s == nil {
		return 0
	}
	size := uint64(headerSize + checksumSize)
	size = addBytes(size, multiplyBytes(uint64(len(s.states)), stateEncodedSize))
	size = addBytes(size, multiplyBytes(uint64(len(s.edges)), edgeEncodedSize))
	size = addBytes(size, multiplyBytes(uint64(len(s.outputs)), 4))
	size = addBytes(size, multiplyBytes(uint64(len(s.ruleIDs)), ruleEncodedSize))
	for _, rule := range s.regexps {
		size = addBytes(size, nonKeywordHeaderSize)
		size = addBytes(size, uint64(len(rule.pattern)))
	}
	if size > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(size)
}

// WriteEncodedTo 将快照按固定小端格式流式写出，并在尾部追加 SHA-256。
func (s *Snapshot) WriteEncodedTo(w io.Writer) (string, error) {
	if s == nil || w == nil {
		return "", errors.New("规则索引写入参数无效")
	}
	hasher := sha256.New()
	payload := io.MultiWriter(w, hasher)
	if err := writeSnapshotPayload(payload, s); err != nil {
		return "", err
	}
	checksum := hasher.Sum(nil)
	if err := writeFull(w, checksum); err != nil {
		return "", err
	}
	return hex.EncodeToString(checksum), nil
}

func writeSnapshotPayload(w io.Writer, s *Snapshot) error {
	if err := writeFull(w, []byte(magic)); err != nil {
		return err
	}
	for _, value := range []any{
		FormatVersion,
		s.version,
		uint32(s.maxMatchIDs),
		uint32(len(s.states)),
		uint32(len(s.edges)),
		uint32(len(s.outputs)),
		uint32(len(s.ruleIDs)),
		uint32(len(s.regexps)),
		uint32(s.stats.KeywordCount),
		uint32(s.stats.RegexpCount),
		uint32(s.stats.CompositeCount),
	} {
		if err := binary.Write(w, binary.LittleEndian, value); err != nil {
			return err
		}
	}

	for _, current := range s.states {
		for _, value := range []uint32{
			current.edgeStart, current.edgeCount, current.failure, current.suffix,
			current.outputStart, current.outputCount,
		} {
			if err := binary.Write(w, binary.LittleEndian, value); err != nil {
				return err
			}
		}
		if err := binary.Write(w, binary.LittleEndian, current.longestAllow); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint16(0)); err != nil {
			return err
		}
	}
	for _, current := range s.edges {
		if err := binary.Write(w, binary.LittleEndian, int32(current.label)); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, current.target); err != nil {
			return err
		}
	}
	for _, output := range s.outputs {
		if err := binary.Write(w, binary.LittleEndian, output); err != nil {
			return err
		}
	}
	for index, id := range s.ruleIDs {
		if err := binary.Write(w, binary.LittleEndian, id); err != nil {
			return err
		}
		if err := writeFull(w, []byte{byte(s.risks[index]), byte(s.effects[index])}); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, s.lengths[index]); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, s.priorities[index]); err != nil {
			return err
		}
	}
	for _, rule := range s.regexps {
		if len(rule.pattern) > math.MaxUint32 {
			return fmt.Errorf("%w: 非关键词模式过大", ErrIndexLimit)
		}
		if err := binary.Write(w, binary.LittleEndian, rule.ruleIndex); err != nil {
			return err
		}
		if err := writeFull(w, []byte{byte(rule.kind), 0, 0, 0}); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint32(len(rule.pattern))); err != nil {
			return err
		}
		if err := writeFull(w, []byte(rule.pattern)); err != nil {
			return err
		}
	}
	return nil
}

// ReadFrom 校验边界与校验和后流式恢复不可变快照。
func ReadFrom(r io.Reader, limits Limits) (*Snapshot, string, error) {
	if r == nil {
		return nil, "", errors.New("规则索引读取参数无效")
	}
	limits, err := normalizeLimits(limits)
	if err != nil {
		return nil, "", err
	}
	hasher := sha256.New()
	payload := io.TeeReader(r, hasher)
	header, err := readCodecHeader(payload)
	if err != nil {
		return nil, "", err
	}
	if err := validateCodecHeader(header, limits); err != nil {
		return nil, "", err
	}

	snapshot, err := allocateDecodedSnapshot(header, limits)
	if err != nil {
		return nil, "", err
	}
	if err := readSnapshotArrays(payload, snapshot, header, limits); err != nil {
		return nil, "", err
	}
	expected := make([]byte, checksumSize)
	if _, err := io.ReadFull(r, expected); err != nil {
		return nil, "", fmt.Errorf("%w: 缺少校验和", ErrIndexCorrupt)
	}
	actual := hasher.Sum(nil)
	if !bytes.Equal(expected, actual) {
		return nil, "", fmt.Errorf("%w: SHA-256 不匹配", ErrIndexCorrupt)
	}
	if err := rejectTrailingBytes(r); err != nil {
		return nil, "", err
	}
	if err := compileDecodedNonKeywords(snapshot, limits); err != nil {
		return nil, "", err
	}
	if err := validateDecodedSnapshot(snapshot, header, limits); err != nil {
		return nil, "", err
	}
	snapshot.stats.IndexBytes = estimateSnapshotBytes(snapshot)
	if snapshot.stats.IndexBytes > limits.MaxIndexMemoryBytes {
		return nil, "", fmt.Errorf("%w: 解码索引常驻超限", ErrIndexLimit)
	}
	return snapshot, hex.EncodeToString(actual), nil
}

func readCodecHeader(r io.Reader) (codecHeader, error) {
	var marker [len(magic)]byte
	if _, err := io.ReadFull(r, marker[:]); err != nil {
		return codecHeader{}, fmt.Errorf("%w: 文件头不完整", ErrIndexCorrupt)
	}
	if string(marker[:]) != magic {
		return codecHeader{}, fmt.Errorf("%w: magic 无效", ErrIndexCorrupt)
	}
	var format uint32
	if err := binary.Read(r, binary.LittleEndian, &format); err != nil || format != FormatVersion {
		return codecHeader{}, fmt.Errorf("%w: 格式版本无效", ErrIndexCorrupt)
	}
	var header codecHeader
	values := []any{
		&header.version,
		&header.maxMatchIDs,
		&header.stateCount,
		&header.edgeCount,
		&header.outputCount,
		&header.ruleCount,
		&header.nonKeywordCount,
		&header.keywordCount,
		&header.regexpCount,
		&header.compositeCount,
	}
	for _, value := range values {
		if err := binary.Read(r, binary.LittleEndian, value); err != nil {
			return codecHeader{}, fmt.Errorf("%w: 文件头不完整", ErrIndexCorrupt)
		}
	}
	return header, nil
}

func validateCodecHeader(header codecHeader, limits Limits) error {
	maxStates := addBytes(1, multiplyBytes(uint64(limits.MaxKeywordRules), uint64(limits.MaxPatternRunes)))
	if uint64(header.stateCount) > maxStates ||
		header.keywordCount > uint32(limits.MaxKeywordRules) ||
		header.nonKeywordCount > uint32(limits.MaxRegexpRules) ||
		header.maxMatchIDs > defaultMaxMatchIDs {
		return fmt.Errorf("%w: 文件声明超过规则上限", ErrIndexLimit)
	}
	if header.stateCount == 0 || header.ruleCount == 0 || header.maxMatchIDs == 0 {
		return fmt.Errorf("%w: 空数组声明", ErrIndexCorrupt)
	}
	if uint64(header.keywordCount)+uint64(header.regexpCount)+uint64(header.compositeCount) != uint64(header.ruleCount) ||
		uint64(header.regexpCount)+uint64(header.compositeCount) != uint64(header.nonKeywordCount) ||
		header.outputCount != header.keywordCount {
		return fmt.Errorf("%w: 规则计数不一致", ErrIndexCorrupt)
	}
	if header.edgeCount != header.stateCount-1 {
		return fmt.Errorf("%w: 自动机数组声明超限", ErrIndexLimit)
	}
	fixedBytes := decodedFixedBytes(header)
	if fixedBytes > limits.MaxIndexMemoryBytes {
		return fmt.Errorf("%w: 声明数组常驻超限", ErrIndexLimit)
	}
	return nil
}

func decodedFixedBytes(header codecHeader) uint64 {
	total := uint64(unsafe.Sizeof(Snapshot{}))
	total = addBytes(total, multiplyBytes(uint64(header.stateCount), uint64(unsafe.Sizeof(state{}))))
	total = addBytes(total, multiplyBytes(uint64(header.edgeCount), uint64(unsafe.Sizeof(edge{}))))
	total = addBytes(total, multiplyBytes(uint64(header.outputCount), 4))
	total = addBytes(total, multiplyBytes(uint64(header.ruleCount), 16))
	total = addBytes(total, multiplyBytes(uint64(header.nonKeywordCount), uint64(unsafe.Sizeof(nonKeywordRule{}))))
	return total
}

func allocateDecodedSnapshot(header codecHeader, limits Limits) (*Snapshot, error) {
	maxMatchIDs := int(header.maxMatchIDs)
	if maxMatchIDs > limits.MaxMatchIDs {
		maxMatchIDs = limits.MaxMatchIDs
	}
	return &Snapshot{
		version:     header.version,
		states:      make([]state, int(header.stateCount)),
		edges:       make([]edge, int(header.edgeCount)),
		outputs:     make([]uint32, int(header.outputCount)),
		ruleIDs:     make([]uint64, int(header.ruleCount)),
		risks:       make([]Risk, int(header.ruleCount)),
		effects:     make([]Effect, int(header.ruleCount)),
		priorities:  make([]int32, int(header.ruleCount)),
		lengths:     make([]uint16, int(header.ruleCount)),
		regexps:     make([]nonKeywordRule, int(header.nonKeywordCount)),
		maxMatchIDs: maxMatchIDs,
		stats: Stats{
			RuleCount: int(header.ruleCount), KeywordCount: int(header.keywordCount),
			RegexpCount: int(header.regexpCount), CompositeCount: int(header.compositeCount),
			StateCount: int(header.stateCount), EdgeCount: int(header.edgeCount), DirectOutputCount: int(header.outputCount),
		},
	}, nil
}

func readSnapshotArrays(r io.Reader, snapshot *Snapshot, header codecHeader, limits Limits) error {
	for index := range snapshot.states {
		current := &snapshot.states[index]
		values := []any{
			&current.edgeStart, &current.edgeCount, &current.failure, &current.suffix,
			&current.outputStart, &current.outputCount, &current.longestAllow,
		}
		for _, value := range values {
			if err := binary.Read(r, binary.LittleEndian, value); err != nil {
				return fmt.Errorf("%w: 状态数组不完整", ErrIndexCorrupt)
			}
		}
		var reserved uint16
		if err := binary.Read(r, binary.LittleEndian, &reserved); err != nil || reserved != 0 {
			return fmt.Errorf("%w: 状态保留字段无效", ErrIndexCorrupt)
		}
	}
	for index := range snapshot.edges {
		var label int32
		if err := binary.Read(r, binary.LittleEndian, &label); err != nil || binary.Read(r, binary.LittleEndian, &snapshot.edges[index].target) != nil {
			return fmt.Errorf("%w: 边数组不完整", ErrIndexCorrupt)
		}
		snapshot.edges[index].label = rune(label)
	}
	for index := range snapshot.outputs {
		if err := binary.Read(r, binary.LittleEndian, &snapshot.outputs[index]); err != nil {
			return fmt.Errorf("%w: 输出数组不完整", ErrIndexCorrupt)
		}
	}
	for index := range snapshot.ruleIDs {
		if err := binary.Read(r, binary.LittleEndian, &snapshot.ruleIDs[index]); err != nil {
			return fmt.Errorf("%w: 规则元数据不完整", ErrIndexCorrupt)
		}
		var compact [2]byte
		if _, err := io.ReadFull(r, compact[:]); err != nil {
			return fmt.Errorf("%w: 规则元数据不完整", ErrIndexCorrupt)
		}
		snapshot.risks[index] = Risk(compact[0])
		snapshot.effects[index] = Effect(compact[1])
		if err := binary.Read(r, binary.LittleEndian, &snapshot.lengths[index]); err != nil ||
			binary.Read(r, binary.LittleEndian, &snapshot.priorities[index]) != nil {
			return fmt.Errorf("%w: 规则元数据不完整", ErrIndexCorrupt)
		}
	}

	patternBudget := decodedFixedBytes(header)
	maxPatternBytes := uint64(limits.MaxPatternRunes * utf8.UTFMax)
	for index := range snapshot.regexps {
		current := &snapshot.regexps[index]
		if err := binary.Read(r, binary.LittleEndian, &current.ruleIndex); err != nil {
			return fmt.Errorf("%w: 非关键词记录不完整", ErrIndexCorrupt)
		}
		var compact [4]byte
		if _, err := io.ReadFull(r, compact[:]); err != nil || compact[1] != 0 || compact[2] != 0 || compact[3] != 0 {
			return fmt.Errorf("%w: 非关键词类型无效", ErrIndexCorrupt)
		}
		current.kind = ruleType(compact[0])
		var patternLength uint32
		if err := binary.Read(r, binary.LittleEndian, &patternLength); err != nil {
			return fmt.Errorf("%w: 非关键词长度缺失", ErrIndexCorrupt)
		}
		if uint64(patternLength) > maxPatternBytes {
			return fmt.Errorf("%w: 非关键词模式过长", ErrIndexLimit)
		}
		patternBudget = addBytes(patternBudget, uint64(patternLength))
		if current.kind == ruleTypeRegexp {
			patternBudget = addBytes(patternBudget, addBytes(256, uint64(patternLength)*4))
		} else if current.kind == ruleTypeComposite {
			patternBudget = addBytes(patternBudget, uint64(patternLength)*4)
		}
		if patternBudget > limits.MaxIndexMemoryBytes {
			return fmt.Errorf("%w: 非关键词模式常驻超限", ErrIndexLimit)
		}
		pattern := make([]byte, int(patternLength))
		if _, err := io.ReadFull(r, pattern); err != nil {
			return fmt.Errorf("%w: 非关键词模式不完整", ErrIndexCorrupt)
		}
		if !utf8.Valid(pattern) || utf8.RuneCount(pattern) > limits.MaxPatternRunes {
			return fmt.Errorf("%w: 非关键词模式无效", ErrIndexCorrupt)
		}
		current.pattern = string(pattern)
	}
	return nil
}

func compileDecodedNonKeywords(snapshot *Snapshot, limits Limits) error {
	for index := range snapshot.regexps {
		current := &snapshot.regexps[index]
		switch current.kind {
		case ruleTypeRegexp:
			compiled, err := textnorm.CompileRegexp(current.pattern)
			if err != nil {
				return fmt.Errorf("%w: 正则模式无效", ErrIndexCorrupt)
			}
			current.regexp = compiled
		case ruleTypeComposite:
			signals, err := compileComposite(current.pattern, limits.MaxPatternRunes)
			if err != nil {
				return fmt.Errorf("%w: 组合模式无效", ErrIndexCorrupt)
			}
			current.signals = signals
		default:
			return fmt.Errorf("%w: 非关键词类型无效", ErrIndexCorrupt)
		}
	}
	return nil
}

func validateDecodedSnapshot(snapshot *Snapshot, header codecHeader, limits Limits) error {
	keywordRules := make([]bool, len(snapshot.ruleIDs))
	nonKeywordRules := make([]bool, len(snapshot.ruleIDs))
	var expectedEdgeStart uint64
	var expectedOutputStart uint64
	for stateIndex, current := range snapshot.states {
		if uint64(current.edgeStart)+uint64(current.edgeCount) > uint64(len(snapshot.edges)) ||
			uint64(current.outputStart)+uint64(current.outputCount) > uint64(len(snapshot.outputs)) ||
			current.failure >= header.stateCount || current.suffix >= header.stateCount ||
			int(current.longestAllow) > limits.MaxPatternRunes {
			return fmt.Errorf("%w: 状态引用越界", ErrIndexCorrupt)
		}
		if uint64(current.edgeStart) != expectedEdgeStart || uint64(current.outputStart) != expectedOutputStart {
			return fmt.Errorf("%w: 状态数组不连续", ErrIndexCorrupt)
		}
		expectedEdgeStart += uint64(current.edgeCount)
		expectedOutputStart += uint64(current.outputCount)
		edges := snapshot.stateEdges(uint32(stateIndex))
		for edgeIndex, currentEdge := range edges {
			if currentEdge.target >= header.stateCount || !utf8.ValidRune(currentEdge.label) ||
				(edgeIndex > 0 && edges[edgeIndex-1].label >= currentEdge.label) {
				return fmt.Errorf("%w: 边引用无效", ErrIndexCorrupt)
			}
		}
	}
	if expectedEdgeStart != uint64(len(snapshot.edges)) || expectedOutputStart != uint64(len(snapshot.outputs)) {
		return fmt.Errorf("%w: 状态数组未覆盖全部数据", ErrIndexCorrupt)
	}
	for _, ruleIndex := range snapshot.outputs {
		if ruleIndex >= header.ruleCount || keywordRules[ruleIndex] {
			return fmt.Errorf("%w: 关键词输出无效", ErrIndexCorrupt)
		}
		keywordRules[ruleIndex] = true
	}
	for _, current := range snapshot.regexps {
		if current.ruleIndex >= header.ruleCount || nonKeywordRules[current.ruleIndex] || keywordRules[current.ruleIndex] {
			return fmt.Errorf("%w: 非关键词引用无效", ErrIndexCorrupt)
		}
		nonKeywordRules[current.ruleIndex] = true
	}
	for index := range snapshot.ruleIDs {
		if snapshot.ruleIDs[index] == 0 || snapshot.risks[index] < RiskLow || snapshot.risks[index] > RiskHigh ||
			snapshot.effects[index] < EffectReview || snapshot.effects[index] > EffectAllow ||
			keywordRules[index] == nonKeywordRules[index] {
			return fmt.Errorf("%w: 规则元数据无效", ErrIndexCorrupt)
		}
		if keywordRules[index] {
			if snapshot.lengths[index] == 0 || int(snapshot.lengths[index]) > limits.MaxPatternRunes {
				return fmt.Errorf("%w: 关键词长度无效", ErrIndexCorrupt)
			}
		} else if snapshot.lengths[index] != 0 || snapshot.effects[index] != EffectReview {
			return fmt.Errorf("%w: 非关键词元数据无效", ErrIndexCorrupt)
		}
	}
	return nil
}

func rejectTrailingBytes(r io.Reader) error {
	var extra [1]byte
	n, err := io.ReadFull(r, extra[:])
	if n > 0 || !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: 文件存在尾随数据", ErrIndexCorrupt)
	}
	return nil
}

func writeFull(w io.Writer, value []byte) error {
	for len(value) > 0 {
		written, err := w.Write(value)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}
