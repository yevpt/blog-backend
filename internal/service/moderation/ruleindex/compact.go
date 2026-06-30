package ruleindex

import (
	"context"
	"sort"
)

func (b *builder) compact(ctx context.Context, version uint64) (*Snapshot, Stats, error) {
	if err := b.checkProjectedIndexBudget(); err != nil {
		return nil, Stats{}, err
	}
	states := make([]state, len(b.states))
	edges := make([]edge, len(b.edges))
	outputs := make([]uint32, len(b.outputs))

	edgeOffset := 0
	outputOffset := 0
	for index, source := range b.states {
		edgeCount := linkedEdgeCount(b.edges, source.firstEdge)
		outputCount := linkedOutputCount(b.outputs, source.firstOutput)
		states[index] = state{
			edgeStart:    uint32(edgeOffset),
			edgeCount:    uint32(edgeCount),
			outputStart:  uint32(outputOffset),
			outputCount:  uint32(outputCount),
			longestAllow: source.directLongestAllow,
		}

		writeEdge := edgeOffset
		for current := source.firstEdge; current != noLink; current = b.edges[current].next {
			edges[writeEdge] = edge{label: b.edges[current].label, target: b.edges[current].target}
			writeEdge++
		}
		sort.Slice(edges[edgeOffset:writeEdge], func(i, j int) bool {
			return edges[edgeOffset+i].label < edges[edgeOffset+j].label
		})

		writeOutput := outputOffset
		for current := source.firstOutput; current != noLink; current = b.outputs[current].next {
			outputs[writeOutput] = b.outputs[current].rule
			writeOutput++
		}
		sort.Slice(outputs[outputOffset:writeOutput], func(i, j int) bool {
			left := outputs[outputOffset+i]
			right := outputs[outputOffset+j]
			return b.ruleIDs[left] < b.ruleIDs[right]
		})

		edgeOffset = writeEdge
		outputOffset = writeOutput
	}

	snapshot := &Snapshot{
		version:     version,
		states:      states,
		edges:       edges,
		outputs:     outputs,
		ruleIDs:     b.ruleIDs,
		risks:       b.risks,
		effects:     b.effects,
		priorities:  b.priorities,
		lengths:     b.lengths,
		regexps:     b.regexps,
		maxMatchIDs: b.limits.MaxMatchIDs,
	}
	if err := snapshot.buildFailureLinks(ctx); err != nil {
		return nil, Stats{}, err
	}

	stats := Stats{
		RuleCount:         len(b.ruleIDs),
		KeywordCount:      b.keywords,
		RegexpCount:       b.regexpCount,
		CompositeCount:    b.composites,
		StateCount:        len(states),
		EdgeCount:         len(edges),
		DirectOutputCount: len(outputs),
	}
	snapshot.stats = stats
	if err := snapshot.finalizeMemoryStats(b); err != nil {
		return nil, Stats{}, err
	}
	return snapshot, snapshot.stats, nil
}

func linkedEdgeCount(edges []buildEdge, first uint32) int {
	count := 0
	for current := first; current != noLink; current = edges[current].next {
		count++
	}
	return count
}

func linkedOutputCount(outputs []buildOutput, first uint32) int {
	count := 0
	for current := first; current != noLink; current = outputs[current].next {
		count++
	}
	return count
}

func (s *Snapshot) buildFailureLinks(ctx context.Context) error {
	queue := make([]uint32, 0, len(s.states))
	for _, rootEdge := range s.stateEdges(0) {
		s.states[rootEdge.target].failure = 0
		s.states[rootEdge.target].suffix = 0
		queue = append(queue, rootEdge.target)
	}

	for head := 0; head < len(queue); head++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := queue[head]
		for _, currentEdge := range s.stateEdges(current) {
			child := currentEdge.target
			failure := s.states[current].failure
			for {
				candidate, found := s.directTransition(failure, currentEdge.label)
				if found && candidate != child {
					failure = candidate
					break
				}
				if failure == 0 {
					break
				}
				failure = s.states[failure].failure
			}

			s.states[child].failure = failure
			if s.states[failure].outputCount > 0 {
				s.states[child].suffix = failure
			} else {
				s.states[child].suffix = s.states[failure].suffix
			}
			if s.states[failure].longestAllow > s.states[child].longestAllow {
				s.states[child].longestAllow = s.states[failure].longestAllow
			}
			queue = append(queue, child)
		}
	}
	return nil
}

func (s *Snapshot) stateEdges(index uint32) []edge {
	current := s.states[index]
	start := int(current.edgeStart)
	return s.edges[start : start+int(current.edgeCount)]
}

func (s *Snapshot) directTransition(stateIndex uint32, label rune) (uint32, bool) {
	edges := s.stateEdges(stateIndex)
	index := sort.Search(len(edges), func(i int) bool { return edges[i].label >= label })
	if index >= len(edges) || edges[index].label != label {
		return 0, false
	}
	return edges[index].target, true
}
