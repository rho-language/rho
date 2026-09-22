package deps

import (
	"fmt"
	"sort"
	"strings"
)

// PrintTree prints the full dependency graph in a readable tree format.
func PrintTree(graph *DependencyGraph) {
	visited := make(map[string]bool)

	names := make([]string, 0, len(graph.Nodes))
	for name := range graph.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		node := graph.Nodes[name]
		if !visited[node.Name] {
			printNode(node, 0, visited)
		}
	}
}

func printNode(node *PackageNode, depth int, visited map[string]bool) {
	visited[node.Name] = true

	prefix := strings.Repeat("  ", depth)
	fmt.Printf("%s- %s %d.%d.%d (%s)\n",
		prefix,
		node.Name,
		node.Version.Major,
		node.Version.Minor,
		node.Version.Patch,
		node.Path,
	)

	for _, child := range node.Deps {
		fmt.Printf("%s  -> %s (%s)\n",
			prefix,
			child.Name,
			child.Version.Raw,
		)
	}
}
