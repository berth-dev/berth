// Package graph provides Knowledge Graph integration.
// This file implements duplication checking to prevent recreating existing functionality.
package graph

import (
	"fmt"
	"strings"
)

// DuplicationResult contains potential duplicate code matches.
type DuplicationResult struct {
	FunctionMatches []string
	TypeMatches     []string
	HasDuplicates   bool
}

// CheckDuplication queries the KG for similar functions or types.
// Returns nil, nil if the client is unavailable (graceful degradation).
func (c *Client) CheckDuplication(funcName, typeName string) (*DuplicationResult, error) {
	if c == nil {
		return nil, nil // KG not available, skip check
	}

	result := &DuplicationResult{}

	// Check for similar functions by querying callers (which proves the function exists).
	if funcName != "" {
		callers, err := c.QueryCallers(funcName)
		if err == nil && len(callers) > 0 {
			// Function exists and has callers - potential duplicate.
			matches := make([]string, 0, len(callers))
			seen := make(map[string]bool)
			for _, caller := range callers {
				if !seen[caller.File] {
					seen[caller.File] = true
					matches = append(matches, fmt.Sprintf("%s:%d", caller.File, caller.Line))
				}
			}
			result.FunctionMatches = matches
			result.HasDuplicates = true
		}
	}

	// Check for similar types by querying type usages (which proves the type exists).
	if typeName != "" {
		usages, err := c.QueryTypeUsages(typeName)
		if err == nil && len(usages) > 0 {
			// Type exists and is used - potential duplicate.
			matches := make([]string, 0, len(usages))
			seen := make(map[string]bool)
			for _, usage := range usages {
				if !seen[usage.File] {
					seen[usage.File] = true
					matches = append(matches, fmt.Sprintf("%s:%d", usage.File, usage.Line))
				}
			}
			result.TypeMatches = matches
			result.HasDuplicates = true
		}
	}

	return result, nil
}

// WarnIfDuplicates logs a warning if duplicates were found.
// This is a non-blocking warning meant to alert developers about potential code duplication.
func WarnIfDuplicates(result *DuplicationResult) {
	if result == nil || !result.HasDuplicates {
		return
	}

	if len(result.FunctionMatches) > 0 {
		fmt.Printf("Warning: similar functions found: %s\n",
			strings.Join(result.FunctionMatches, ", "))
	}
	if len(result.TypeMatches) > 0 {
		fmt.Printf("Warning: similar types found: %s\n",
			strings.Join(result.TypeMatches, ", "))
	}
}

// CheckDuplicationFromTitle extracts potential function/type names from a bead title
// and checks for duplicates. This is a convenience wrapper that handles name extraction.
// Common patterns: "add FuncName function", "create TypeName type", "implement Handler".
func (c *Client) CheckDuplicationFromTitle(title string) (*DuplicationResult, error) {
	if c == nil || title == "" {
		return nil, nil
	}

	funcName, typeName := extractNamesFromTitle(title)
	if funcName == "" && typeName == "" {
		return nil, nil
	}

	return c.CheckDuplication(funcName, typeName)
}

// extractNamesFromTitle attempts to extract function and type names from a bead title.
// It looks for common patterns like "add X function", "create X type", "implement X".
// Returns empty strings if no names can be extracted.
// Names are returned in lowercase for consistent matching.
func extractNamesFromTitle(title string) (funcName, typeName string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ""
	}

	// Keep original words for capitalization check, but use lowercase for verb matching.
	originalWords := strings.Fields(title)
	if len(originalWords) < 2 {
		return "", ""
	}

	// Look for patterns: "add/create/implement X function/type/handler/struct/interface"
	for i := 0; i < len(originalWords)-1; i++ {
		verb := strings.ToLower(originalWords[i])
		originalName := originalWords[i+1]
		name := extractIdentifierFromWord(originalName)

		// Skip common verbs to get to the actual name.
		if verb == "add" || verb == "create" || verb == "implement" || verb == "define" {
			if name == "" {
				continue
			}

			// Check if there's a type hint after the name.
			if i+2 < len(originalWords) {
				typeHint := strings.ToLower(originalWords[i+2])
				switch typeHint {
				case "function", "func", "method", "handler":
					funcName = strings.ToLower(name)
				case "type", "struct", "interface", "class":
					typeName = strings.ToLower(name)
				default:
					// Default: treat as function if name looks like it.
					if strings.HasSuffix(strings.ToLower(name), "er") || strings.HasSuffix(strings.ToLower(name), "or") {
						funcName = strings.ToLower(name)
					} else if isCapitalized(name) {
						typeName = strings.ToLower(name)
					} else {
						funcName = strings.ToLower(name)
					}
				}
			} else {
				// No type hint; use heuristics based on capitalization.
				if isCapitalized(name) {
					typeName = strings.ToLower(name)
				} else {
					funcName = strings.ToLower(name)
				}
			}
			return
		}
	}

	return "", ""
}

// extractIdentifierFromWord cleans a word to extract a valid identifier.
func extractIdentifierFromWord(word string) string {
	var b strings.Builder
	for _, r := range word {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	return b.String()
}

// isCapitalized checks if a string starts with an uppercase letter.
func isCapitalized(s string) bool {
	if s == "" {
		return false
	}
	r := rune(s[0])
	return r >= 'A' && r <= 'Z'
}
