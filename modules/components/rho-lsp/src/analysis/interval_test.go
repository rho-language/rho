package analysis

import "testing"

func TestIntervalIndexFindsNode(t *testing.T) {
	module, _ := Parse(Lex("module demo\nfn main() void"))
	index := BuildIntervalIndex(module.Nodes)
	if node := index.NodeAt(1, 4); node == nil || node.Name() != "main" {
		t.Fatalf("expected main node, got %#v", node)
	}
	if node := index.NodeAt(4, 0); node != nil {
		t.Fatalf("expected no node, got %#v", node)
	}
}
