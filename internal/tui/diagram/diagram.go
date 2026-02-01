package diagram

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/berth-dev/berth/internal/graph"
)

// GenerateASCII creates an ASCII architecture diagram from nodes
func GenerateASCII(nodes map[string]graph.ArchitectureNode) string {
	var b strings.Builder

	b.WriteString("Architecture Diagram\n")
	b.WriteString("====================\n\n")

	// Group nodes by depth
	layers := make(map[int][]string)
	for file, node := range nodes {
		layers[node.Depth] = append(layers[node.Depth], file)
	}

	// Get sorted depth levels
	depths := make([]int, 0, len(layers))
	for d := range layers {
		depths = append(depths, d)
	}
	sort.Ints(depths)

	// Render each layer
	for _, depth := range depths {
		files := layers[depth]
		sort.Strings(files) // deterministic order

		b.WriteString(fmt.Sprintf("Layer %d:\n", depth))

		indent := strings.Repeat("  ", depth)
		for _, file := range files {
			node := nodes[file]
			shortFile := shortPath(file)
			exports := truncateList(node.Exports, 3)

			line := fmt.Sprintf("%s├── %s", indent, shortFile)
			if exports != "" {
				line += fmt.Sprintf(" [%s]", exports)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// shortPath shortens file path for display
func shortPath(file string) string {
	// Return just filename and parent dir
	dir := filepath.Base(filepath.Dir(file))
	base := filepath.Base(file)
	if dir == "." || dir == "/" {
		return base
	}
	return filepath.Join(dir, base)
}

// truncateList returns first N items joined
func truncateList(items []string, max int) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + "..."
}
