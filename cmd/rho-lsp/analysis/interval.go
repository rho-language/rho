package analysis

import "sort"

type intervalEntry struct {
	start, end int
	node       Node
}
type IntervalIndex struct{ byLine map[int][]intervalEntry }

func BuildIntervalIndex(nodes []Node) *IntervalIndex {
	index := &IntervalIndex{byLine: map[int][]intervalEntry{}}
	for _, node := range nodes {
		line := node.Line()
		index.byLine[line] = append(index.byLine[line], intervalEntry{node.Column(), node.Column() + len([]rune(node.Name())), node})
	}
	for line := range index.byLine {
		sort.Slice(index.byLine[line], func(i, j int) bool { return index.byLine[line][i].start < index.byLine[line][j].start })
	}
	return index
}

func (index *IntervalIndex) NodeAt(line, column int) Node {
	entries := index.byLine[line]
	position := sort.Search(len(entries), func(i int) bool { return entries[i].end >= column })
	if position < len(entries) && column >= entries[position].start && column <= entries[position].end {
		return entries[position].node
	}
	return nil
}
