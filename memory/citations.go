package memory

import (
	"fmt"
	"path/filepath"
	"strings"
)

// formatCitation formats a citation string for a memory entry
// Format: path/to/file.md#L10-L20 (includes end line if available)
func formatCitation(ve *VectorEmbedding) string {
	var parts []string

	// Add file path
	if ve.Metadata.FilePath != "" {
		parts = append(parts, ve.Metadata.FilePath)
	} else if ve.Metadata.SessionKey != "" {
		parts = append(parts, ve.Metadata.SessionKey)
	}

	// Add line numbers
	startLine := ve.Metadata.LineNumber
	endLine := ve.Metadata.EndLineNumber

	if startLine > 0 {
		if endLine > 0 && endLine != startLine {
			parts = append(parts, fmt.Sprintf("L%d-L%d", startLine, endLine))
		} else {
			parts = append(parts, fmt.Sprintf("L%d", startLine))
		}
	}

	if len(parts) == 0 {
		return ve.ID
	}

	return strings.Join(parts, "#")
}

// formatSnippetWithCitation appends citation to a snippet
func formatSnippetWithCitation(snippet string, ve *VectorEmbedding) string {
	citation := formatCitation(ve)
	if citation == "" {
		return snippet
	}
	return fmt.Sprintf("%s\n\nSource: %s", snippet, citation)
}

// formatResultsWithCitations adds citations to search results
func formatResultsWithCitations(results []*SearchResult) []*SearchResult {
	for _, r := range results {
		r.Citation = formatCitation(&r.VectorEmbedding)
		// Update snippet to include citation
		if r.Text != "" && r.Citation != "" {
			r.Text = fmt.Sprintf("%s\n\nSource: %s", r.Text, r.Citation)
		}
	}
	return results
}

// FormatCitationForDisplay formats citation for display in responses
// Returns empty string if citations should be hidden
func FormatCitationForDisplay(ve *VectorEmbedding, mode CitationsMode, isGroupChat bool) string {
	switch mode {
	case CitationsModeOff:
		return ""
	case CitationsModeOn:
		return formatCitation(ve)
	case CitationsModeAuto:
		// Only show in direct chats, not in group chats
		if isGroupChat {
			return ""
		}
		return formatCitation(ve)
	default:
		return formatCitation(ve)
	}
}

// DecorateResultsWithCitations decorates search results with citations based on mode
func DecorateResultsWithCitations(results []*SearchResult, mode CitationsMode, isGroupChat bool) []*SearchResult {
	if mode == CitationsModeOff || (mode == CitationsModeAuto && isGroupChat) {
		// Strip citations from text
		for _, r := range results {
			if r.Citation != "" {
				r.Text = strings.Split(r.Text, "\n\nSource:")[0]
				r.Citation = ""
			}
		}
		return results
	}

	// Add citations
	for _, r := range results {
		citation := FormatCitationForDisplay(&r.VectorEmbedding, mode, isGroupChat)
		r.Citation = citation
		if citation != "" && r.Text != "" {
			if !strings.Contains(r.Text, "\n\nSource:") {
				r.Text = fmt.Sprintf("%s\n\nSource: %s", r.Text, citation)
			}
		}
	}
	return results
}

// BuildRelativePath makes file path relative to workspace if possible
func BuildRelativePath(filePath, workspaceDir string) string {
	if workspaceDir == "" {
		return filePath
	}

	// Try to make relative
	rel, err := filepath.Rel(workspaceDir, filePath)
	if err != nil {
		return filePath
	}

	// If the relative path is shorter or starts with "..", use original
	if len(rel) < len(filePath) && !strings.HasPrefix(rel, "..") {
		return rel
	}

	return filePath
}

// GetCitationLineRange returns the line range for citation display
func GetCitationLineRange(ve *VectorEmbedding) string {
	startLine := ve.Metadata.LineNumber
	endLine := ve.Metadata.EndLineNumber

	if startLine <= 0 {
		return ""
	}

	if endLine > 0 && endLine != startLine {
		return fmt.Sprintf("L%d-L%d", startLine, endLine)
	}
	return fmt.Sprintf("L%d", startLine)
}
