package plan

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var frameworkPackages = map[string][]string{
	"next":    {"next", "react", "react-dom", "tailwindcss", "typescript", "better-sqlite3"},
	"react":   {"react", "react-dom", "typescript"},
	"vue":     {"vue", "typescript", "vite"},
	"svelte":  {"svelte", "@sveltejs/kit", "typescript"},
	"express": {"express", "typescript"},
}

// DiscoverVersions queries the npm registry for latest versions of
// framework packages. Best-effort: returns empty string on any failure.
func DiscoverVersions(framework string) string {
	packages, ok := frameworkPackages[framework]
	if !ok || len(packages) == 0 {
		return ""
	}

	var lines []string
	for _, pkg := range packages {
		ver := npmLatest(pkg)
		if ver != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s (latest)", pkg, ver))
		}
	}
	if len(lines) == 0 {
		return ""
	}

	return "## Current Package Versions (live from npm registry)\n\n" +
		strings.Join(lines, "\n") + "\n\n" +
		"Use these versions or newer. NEVER use older versions from memory.\n\n"
}

func npmLatest(pkg string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npm", "view", pkg, "version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
