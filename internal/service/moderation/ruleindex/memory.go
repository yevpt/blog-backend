package ruleindex

import (
	"fmt"
	"math"
	"unsafe"
)

const memorySafetyPercent = 20

func (b *builder) checkBuildPeakBudget() error {
	if b.limits.MaxBuildPeakMemoryBytes == 0 {
		return nil
	}
	peak := estimateBuildPeak(b, nil)
	if peak > b.limits.MaxBuildPeakMemoryBytes {
		return fmt.Errorf("%w: 构建峰值 %d 超过 %d", ErrIndexLimit, peak, b.limits.MaxBuildPeakMemoryBytes)
	}
	return nil
}

func (b *builder) checkProjectedIndexBudget() error {
	projected := estimateProjectedSnapshotBytes(b)
	if projected > b.limits.MaxIndexMemoryBytes {
		return fmt.Errorf("%w: 索引常驻 %d 超过 %d", ErrIndexLimit, projected, b.limits.MaxIndexMemoryBytes)
	}
	return nil
}

func (s *Snapshot) finalizeMemoryStats(b *builder) error {
	indexBytes := estimateSnapshotBytes(s)
	peakBytes := estimateBuildPeak(b, s)
	if b.limits.MaxIndexMemoryBytes > 0 && indexBytes > b.limits.MaxIndexMemoryBytes {
		return fmt.Errorf("%w: 索引常驻 %d 超过 %d", ErrIndexLimit, indexBytes, b.limits.MaxIndexMemoryBytes)
	}
	if b.limits.MaxBuildPeakMemoryBytes > 0 && peakBytes > b.limits.MaxBuildPeakMemoryBytes {
		return fmt.Errorf("%w: 构建峰值 %d 超过 %d", ErrIndexLimit, peakBytes, b.limits.MaxBuildPeakMemoryBytes)
	}
	s.stats.IndexBytes = indexBytes
	s.stats.BuildPeakBytes = peakBytes
	return nil
}

func estimateSnapshotBytes(s *Snapshot) uint64 {
	if s == nil {
		return 0
	}
	total := uint64(unsafe.Sizeof(*s))
	total = addBytes(total, sliceBytes(cap(s.states), unsafe.Sizeof(state{})))
	total = addBytes(total, sliceBytes(cap(s.edges), unsafe.Sizeof(edge{})))
	total = addBytes(total, sliceBytes(cap(s.outputs), unsafe.Sizeof(uint32(0))))
	total = addBytes(total, sliceBytes(cap(s.ruleIDs), unsafe.Sizeof(uint64(0))))
	total = addBytes(total, sliceBytes(cap(s.risks), unsafe.Sizeof(Risk(0))))
	total = addBytes(total, sliceBytes(cap(s.effects), unsafe.Sizeof(Effect(0))))
	total = addBytes(total, sliceBytes(cap(s.priorities), unsafe.Sizeof(int32(0))))
	total = addBytes(total, sliceBytes(cap(s.lengths), unsafe.Sizeof(uint16(0))))
	total = addBytes(total, nonKeywordBytes(s.regexps))
	return total
}

func estimateProjectedSnapshotBytes(b *builder) uint64 {
	total := uint64(unsafe.Sizeof(Snapshot{}))
	total = addBytes(total, sliceBytes(len(b.states), unsafe.Sizeof(state{})))
	total = addBytes(total, sliceBytes(len(b.edges), unsafe.Sizeof(edge{})))
	total = addBytes(total, sliceBytes(len(b.outputs), unsafe.Sizeof(uint32(0))))
	total = addBytes(total, sliceBytes(cap(b.ruleIDs), unsafe.Sizeof(uint64(0))))
	total = addBytes(total, sliceBytes(cap(b.risks), unsafe.Sizeof(Risk(0))))
	total = addBytes(total, sliceBytes(cap(b.effects), unsafe.Sizeof(Effect(0))))
	total = addBytes(total, sliceBytes(cap(b.priorities), unsafe.Sizeof(int32(0))))
	total = addBytes(total, sliceBytes(cap(b.lengths), unsafe.Sizeof(uint16(0))))
	total = addBytes(total, nonKeywordBytes(b.regexps))
	return total
}

func estimateBuildPeak(b *builder, snapshot *Snapshot) uint64 {
	total := b.limits.CurrentIndexMemoryBytes
	total = addBytes(total, uint64(unsafe.Sizeof(*b)))
	total = addBytes(total, sliceBytes(cap(b.states), unsafe.Sizeof(buildState{})))
	total = addBytes(total, sliceBytes(cap(b.edges), unsafe.Sizeof(buildEdge{})))
	total = addBytes(total, sliceBytes(cap(b.outputs), unsafe.Sizeof(buildOutput{})))
	total = addBytes(total, sliceBytes(cap(b.ruleIDs), unsafe.Sizeof(uint64(0))))
	total = addBytes(total, sliceBytes(cap(b.risks), unsafe.Sizeof(Risk(0))))
	total = addBytes(total, sliceBytes(cap(b.effects), unsafe.Sizeof(Effect(0))))
	total = addBytes(total, sliceBytes(cap(b.priorities), unsafe.Sizeof(int32(0))))
	total = addBytes(total, sliceBytes(cap(b.lengths), unsafe.Sizeof(uint16(0))))
	total = addBytes(total, nonKeywordBytes(b.regexps))
	total = addBytes(total, mapBytes(len(b.transitions), unsafe.Sizeof(transitionKey{}), unsafe.Sizeof(uint32(0))))
	total = addBytes(total, mapBytes(len(b.seenIDs), unsafe.Sizeof(uint64(0)), 0))

	if snapshot == nil {
		total = addBytes(total, sliceBytes(len(b.states), unsafe.Sizeof(state{})))
		total = addBytes(total, sliceBytes(len(b.edges), unsafe.Sizeof(edge{})))
		total = addBytes(total, sliceBytes(len(b.outputs), unsafe.Sizeof(uint32(0))))
	} else {
		total = addBytes(total, estimateSnapshotBytes(snapshot))
	}
	return addSafetyMargin(total)
}

func nonKeywordBytes(rules []nonKeywordRule) uint64 {
	total := sliceBytes(cap(rules), unsafe.Sizeof(nonKeywordRule{}))
	for _, rule := range rules {
		total = addBytes(total, uint64(len(rule.pattern)))
		total = addBytes(total, sliceBytes(cap(rule.signals), unsafe.Sizeof("")))
		for _, signal := range rule.signals {
			total = addBytes(total, uint64(len(signal)))
		}
		if rule.regexp != nil {
			// RE2 内部节点不暴露容量，按模式字节四倍加固定余量保守估算。
			total = addBytes(total, addBytes(256, uint64(len(rule.pattern))*4))
		}
	}
	return total
}

func sliceBytes(capacity int, elementSize uintptr) uint64 {
	return multiplyBytes(uint64(capacity), uint64(elementSize))
}

func mapBytes(length int, keySize, valueSize uintptr) uint64 {
	// Go map 桶布局不稳定，按键值大小外加每项 16 字节控制开销估算。
	perEntry := addBytes(uint64(keySize), uint64(valueSize))
	perEntry = addBytes(perEntry, 16)
	return multiplyBytes(uint64(length), perEntry)
}

func addSafetyMargin(value uint64) uint64 {
	margin := value / 100 * memorySafetyPercent
	remainderMargin := value % 100 * memorySafetyPercent / 100
	margin = addBytes(margin, remainderMargin)
	if value > 0 && margin == 0 {
		margin = 1
	}
	return addBytes(value, margin)
}

func addBytes(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

func multiplyBytes(left, right uint64) uint64 {
	if left != 0 && right > math.MaxUint64/left {
		return math.MaxUint64
	}
	return left * right
}
