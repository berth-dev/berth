// embed.go pre-embeds graph data by querying SQLite directly for bead prompts.
package graph

import (
	"database/sql"
	"fmt"
	"strings"
)

// GraphData holds pre-embedded Knowledge Graph data for a set of files.
type GraphData struct {
	Files  []FileGraphData
	Impact *ImpactAnalysis
}

// FileGraphData holds graph data for a single file.
type FileGraphData struct {
	Path       string
	Exports    []ExportResult
	Importers  []ImporterResult
	Callers    map[string][]CallerResult    // function name -> callers
	TypeUsages map[string][]TypeUsageResult // type name -> usages
}

// QueryGraphForFiles queries the SQLite database directly (not via MCP) to
// build graph data for the specified files. This is used to pre-embed context
// into bead prompts.
func QueryGraphForFiles(db *sql.DB, files []string) (*GraphData, error) {
	data := &GraphData{
		Files: make([]FileGraphData, 0, len(files)),
	}

	for _, file := range files {
		fgd, err := queryFileGraphData(db, file)
		if err != nil {
			return nil, fmt.Errorf("graph: querying graph data for %s: %w", file, err)
		}
		data.Files = append(data.Files, *fgd)
	}

	return data, nil
}

// queryFileGraphData builds the graph data for a single file.
func queryFileGraphData(db *sql.DB, file string) (*FileGraphData, error) {
	fgd := &FileGraphData{
		Path:       file,
		Callers:    make(map[string][]CallerResult),
		TypeUsages: make(map[string][]TypeUsageResult),
	}

	// Query exports.
	exports, err := queryExports(db, file)
	if err != nil {
		return nil, err
	}
	fgd.Exports = exports

	// Query importers.
	importers, err := queryImporters(db, file)
	if err != nil {
		return nil, err
	}
	fgd.Importers = importers

	// For each exported function, get callers.
	for _, exp := range exports {
		if exp.Kind == "function" {
			callers, err := queryCallers(db, exp.Name, file)
			if err != nil {
				return nil, err
			}
			if len(callers) > 0 {
				fgd.Callers[exp.Name] = callers
			}
		}
	}

	// For each exported type, get usages.
	for _, exp := range exports {
		if exp.Kind == "type" || exp.Kind == "class" || exp.Kind == "interface" || exp.Kind == "enum" {
			usages, err := queryTypeUsages(db, exp.Name, file)
			if err != nil {
				return nil, err
			}
			if len(usages) > 0 {
				fgd.TypeUsages[exp.Name] = usages
			}
		}
	}

	return fgd, nil
}

// queryExports retrieves exported symbols for a file from the symbols table.
func queryExports(db *sql.DB, file string) ([]ExportResult, error) {
	rows, err := db.Query(
		"SELECT name, kind, line FROM symbols WHERE file = ? AND exported = true",
		file,
	)
	if err != nil {
		return nil, fmt.Errorf("querying exports: %w", err)
	}
	defer rows.Close()

	var results []ExportResult
	for rows.Next() {
		var r ExportResult
		if err := rows.Scan(&r.Name, &r.Kind, &r.Line); err != nil {
			return nil, fmt.Errorf("scanning export row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// queryImporters retrieves files that import from the specified file.
func queryImporters(db *sql.DB, file string) ([]ImporterResult, error) {
	rows, err := db.Query(
		"SELECT source_file, imported_names FROM imports WHERE target_file = ?",
		file,
	)
	if err != nil {
		return nil, fmt.Errorf("querying importers: %w", err)
	}
	defer rows.Close()

	var results []ImporterResult
	for rows.Next() {
		var r ImporterResult
		var namesStr string
		if err := rows.Scan(&r.File, &namesStr); err != nil {
			return nil, fmt.Errorf("scanning importer row: %w", err)
		}
		if namesStr != "" {
			r.ImportedNames = strings.Split(namesStr, ",")
			for i := range r.ImportedNames {
				r.ImportedNames[i] = strings.TrimSpace(r.ImportedNames[i])
			}
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// queryCallers retrieves callers of a function from the references table.
func queryCallers(db *sql.DB, name, file string) ([]CallerResult, error) {
	rows, err := db.Query(
		`SELECT r.referenced_in, r.line, COALESCE(s2.name, '')
		 FROM references r
		 JOIN symbols s ON r.symbol_id = s.id
		 LEFT JOIN symbols s2 ON s2.file = r.referenced_in AND s2.line = r.line
		 WHERE s.name = ? AND s.file = ? AND r.ref_kind = 'call'`,
		name, file,
	)
	if err != nil {
		return nil, fmt.Errorf("querying callers: %w", err)
	}
	defer rows.Close()

	var results []CallerResult
	for rows.Next() {
		var r CallerResult
		if err := rows.Scan(&r.File, &r.Line, &r.Name); err != nil {
			return nil, fmt.Errorf("scanning caller row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// queryTypeUsages retrieves usage locations for a type from the references table.
func queryTypeUsages(db *sql.DB, name, file string) ([]TypeUsageResult, error) {
	rows, err := db.Query(
		`SELECT r.referenced_in, r.line, r.ref_kind
		 FROM references r
		 JOIN symbols s ON r.symbol_id = s.id
		 WHERE s.name = ? AND s.file = ? AND r.ref_kind = 'type_usage'`,
		name, file,
	)
	if err != nil {
		return nil, fmt.Errorf("querying type usages: %w", err)
	}
	defer rows.Close()

	var results []TypeUsageResult
	for rows.Next() {
		var r TypeUsageResult
		if err := rows.Scan(&r.File, &r.Line, &r.Kind); err != nil {
			return nil, fmt.Errorf("scanning type usage row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// FormatGraphData formats GraphData as markdown for embedding in bead prompts.
// Returns an empty string if there is no data.
func FormatGraphData(data *GraphData) string {
	if data == nil || len(data.Files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Code Context (pre-embedded from Knowledge Graph)\n")

	hasContent := false
	for _, f := range data.Files {
		if len(f.Exports) == 0 && len(f.Importers) == 0 && len(f.Callers) == 0 && len(f.TypeUsages) == 0 {
			continue
		}
		hasContent = true

		b.WriteString("### ")
		b.WriteString(f.Path)
		b.WriteString("\n")

		// Exports.
		if len(f.Exports) > 0 {
			b.WriteString("- Exports: ")
			names := make([]string, 0, len(f.Exports))
			for _, e := range f.Exports {
				names = append(names, e.Name)
			}
			b.WriteString(strings.Join(names, ", "))
			b.WriteString("\n")
		}

		// Importers.
		if len(f.Importers) > 0 {
			b.WriteString("- Imported by: ")
			files := make([]string, 0, len(f.Importers))
			for _, imp := range f.Importers {
				files = append(files, imp.File)
			}
			b.WriteString(strings.Join(files, ", "))
			b.WriteString("\n")
		}

		// Callers.
		for funcName, callers := range f.Callers {
			b.WriteString("- ")
			b.WriteString(funcName)
			b.WriteString("() callers: ")
			parts := make([]string, 0, len(callers))
			for _, c := range callers {
				if c.Name != "" {
					parts = append(parts, fmt.Sprintf("called from %s:%d (%s)", c.File, c.Line, c.Name))
				} else {
					parts = append(parts, fmt.Sprintf("called from %s:%d", c.File, c.Line))
				}
			}
			b.WriteString(strings.Join(parts, ", "))
			b.WriteString("\n")
		}

		// Type usages.
		for typeName, usages := range f.TypeUsages {
			b.WriteString("- ")
			b.WriteString(typeName)
			b.WriteString(" usages: ")
			parts := make([]string, 0, len(usages))
			for _, u := range usages {
				parts = append(parts, fmt.Sprintf("used in %s:%d", u.File, u.Line))
			}
			b.WriteString(strings.Join(parts, ", "))
			b.WriteString("\n")
		}
	}

	// Impact analysis section.
	if data.Impact != nil && (len(data.Impact.DirectDependents) > 0 || len(data.Impact.TransitiveDependents) > 0 || len(data.Impact.AffectedTests) > 0) {
		hasContent = true
		b.WriteString("\n### Impact Analysis\n")
		b.WriteString("Changing these files may affect:\n")
		if len(data.Impact.DirectDependents) > 0 {
			b.WriteString("- Direct dependents: ")
			parts := make([]string, 0, len(data.Impact.DirectDependents))
			for _, d := range data.Impact.DirectDependents {
				parts = append(parts, fmt.Sprintf("%s (%s via %s)", d.File, d.Kind, d.Name))
			}
			b.WriteString(strings.Join(parts, ", "))
			b.WriteString("\n")
		}
		if len(data.Impact.TransitiveDependents) > 0 {
			b.WriteString("- Transitive dependents: ")
			parts := make([]string, 0, len(data.Impact.TransitiveDependents))
			for _, t := range data.Impact.TransitiveDependents {
				parts = append(parts, fmt.Sprintf("%s (via %s)", t.File, t.Via))
			}
			b.WriteString(strings.Join(parts, ", "))
			b.WriteString("\n")
		}
		if len(data.Impact.AffectedTests) > 0 {
			b.WriteString("- Affected tests: ")
			b.WriteString(strings.Join(data.Impact.AffectedTests, ", "))
			b.WriteString("\n")
		}
	}

	if !hasContent {
		return ""
	}

	return b.String()
}
