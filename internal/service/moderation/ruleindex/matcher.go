package ruleindex

import (
	"strings"
)

// Match 在已归一化文本上执行关键词、正则和组合规则匹配。
func (s *Snapshot) Match(normalized string) MatchResult {
	if s == nil || len(s.states) == 0 {
		return MatchResult{Risk: RiskLow}
	}

	runes := []rune(normalized)
	uncoveredPrefix := s.buildUncoveredPrefix(runes)
	retained := matchCollector{snapshot: s, risk: RiskLow, indexes: make([]uint32, 0, s.maxMatchIDs)}
	suppressed := make([]uint32, 0, min(s.maxMatchIDs, 8))

	current := uint32(0)
	for end, value := range runes {
		current = s.advance(current, value)
		for outputState := current; outputState != 0; outputState = s.states[outputState].suffix {
			for _, ruleIndex := range s.stateOutputs(outputState) {
				if s.effects[ruleIndex] == EffectAllow {
					continue
				}
				length := int(s.lengths[ruleIndex])
				start := end + 1 - length
				if start >= 0 && fullyCovered(uncoveredPrefix, start, end+1) {
					suppressed = s.addSuppressed(suppressed, ruleIndex)
					continue
				}
				retained.add(ruleIndex)
			}
		}
	}

	for _, rule := range s.regexps {
		if rule.matches(normalized) {
			retained.add(rule.ruleIndex)
		}
	}

	return MatchResult{
		Risk:          retained.risk,
		RuleIDs:       s.ruleIndexesToIDs(retained.indexes),
		SuppressedIDs: s.ruleIndexesToIDs(suppressed),
		Truncated:     retained.truncated,
	}
}

func (s *Snapshot) buildUncoveredPrefix(text []rune) []uint32 {
	delta := make([]int32, len(text)+1)
	current := uint32(0)
	for end, value := range text {
		current = s.advance(current, value)
		length := int(s.states[current].longestAllow)
		if length == 0 {
			continue
		}
		start := end + 1 - length
		if start < 0 {
			start = 0
		}
		delta[start]++
		delta[end+1]--
	}

	prefix := make([]uint32, len(text)+1)
	var coverage int32
	for index := range text {
		coverage += delta[index]
		prefix[index+1] = prefix[index]
		if coverage == 0 {
			prefix[index+1]++
		}
	}
	return prefix
}

func fullyCovered(uncoveredPrefix []uint32, start, end int) bool {
	return uncoveredPrefix[end] == uncoveredPrefix[start]
}

func (s *Snapshot) advance(current uint32, label rune) uint32 {
	for {
		if next, found := s.directTransition(current, label); found {
			return next
		}
		if current == 0 {
			return 0
		}
		current = s.states[current].failure
	}
}

func (s *Snapshot) stateOutputs(index uint32) []uint32 {
	current := s.states[index]
	start := int(current.outputStart)
	return s.outputs[start : start+int(current.outputCount)]
}

func (r nonKeywordRule) matches(text string) bool {
	switch r.kind {
	case ruleTypeRegexp:
		return r.regexp != nil && r.regexp.MatchString(text)
	case ruleTypeComposite:
		if len(r.signals) == 0 {
			return false
		}
		for _, signal := range r.signals {
			if !strings.Contains(text, signal) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

type matchCollector struct {
	snapshot  *Snapshot
	indexes   []uint32
	risk      Risk
	truncated bool
}

func (c *matchCollector) add(ruleIndex uint32) {
	risk := c.snapshot.risks[ruleIndex]
	if risk > c.risk {
		c.risk = risk
	}
	for _, retained := range c.indexes {
		if retained == ruleIndex {
			return
		}
	}
	if len(c.indexes) < c.snapshot.maxMatchIDs {
		c.indexes = append(c.indexes, ruleIndex)
		c.sort()
		return
	}

	c.truncated = true
	worst := len(c.indexes) - 1
	if worst >= 0 && c.snapshot.betterRule(ruleIndex, c.indexes[worst]) {
		c.indexes[worst] = ruleIndex
		c.sort()
	}
}

func (c *matchCollector) sort() {
	c.snapshot.sortRuleIndexes(c.indexes)
}

func (s *Snapshot) addSuppressed(current []uint32, ruleIndex uint32) []uint32 {
	for _, existing := range current {
		if existing == ruleIndex {
			return current
		}
	}
	if len(current) >= s.maxMatchIDs {
		return current
	}
	current = append(current, ruleIndex)
	s.sortRuleIndexes(current)
	return current
}

func (s *Snapshot) sortRuleIndexes(indexes []uint32) {
	// 保留集合最多 128 项，插入排序避免反射排序在每次命中时分配。
	for index := 1; index < len(indexes); index++ {
		value := indexes[index]
		position := index
		for position > 0 && s.betterRule(value, indexes[position-1]) {
			indexes[position] = indexes[position-1]
			position--
		}
		indexes[position] = value
	}
}

func (s *Snapshot) betterRule(left, right uint32) bool {
	if s.risks[left] != s.risks[right] {
		return s.risks[left] > s.risks[right]
	}
	if s.priorities[left] != s.priorities[right] {
		return s.priorities[left] < s.priorities[right]
	}
	return s.ruleIDs[left] < s.ruleIDs[right]
}

func (s *Snapshot) ruleIndexesToIDs(indexes []uint32) []uint64 {
	ids := make([]uint64, len(indexes))
	for index, ruleIndex := range indexes {
		ids[index] = s.ruleIDs[ruleIndex]
	}
	return ids
}
