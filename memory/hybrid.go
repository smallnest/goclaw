package memory

import (
	"strings"
	"unicode"
)

// HybridVectorResult represents a vector search result
type HybridVectorResult struct {
	ID          string
	Path        string
	StartLine   int
	EndLine     int
	Source      string
	Snippet     string
	VectorScore float64
}

// HybridKeywordResult represents a keyword/FTS search result
type HybridKeywordResult struct {
	ID        string
	Path      string
	StartLine int
	EndLine   int
	Source    string
	Snippet   string
	TextScore float64
}

// hybridItem represents an item in the merged results
type hybridItem struct {
	ID          string
	Path        string
	StartLine   int
	EndLine     int
	Source      string
	Snippet     string
	VectorScore float64
	TextScore   float64
}

// BuildFTSQuery builds an FTS query from raw text
// Extracts tokens and creates a quoted AND query
func BuildFTSQuery(raw string) string {
	// Extract alphanumeric tokens
	var tokens []string
	var currentToken strings.Builder

	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			currentToken.WriteRune(r)
		} else {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
		}
	}
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	if len(tokens) == 0 {
		return ""
	}

	// Quote each token and join with AND
	var quoted strings.Builder
	for i, token := range tokens {
		if i > 0 {
			quoted.WriteString(" AND ")
		}
		quoted.WriteString(`"`)
		quoted.WriteString(strings.ReplaceAll(token, `"`, `""`))
		quoted.WriteString(`"`)
	}

	return quoted.String()
}

// BM25RankToScore converts BM25 rank to similarity score (0-1)
// Higher rank = lower score, using 1/(1+rank) formula
func BM25RankToScore(rank float64) float64 {
	if !isFinite(rank) || rank < 0 {
		rank = 999
	}
	return 1.0 / (1.0 + rank)
}

func isFinite(f float64) bool {
	return !isInf(f, 0) && !isNaN(f)
}

func isInf(f float64, sign int) bool {
	// Simple check for infinity
	return f > 1.7976931348623157e308 || f < -1.7976931348623157e308
}

func isNaN(f float64) bool {
	return f != f
}

// MergeHybridResults merges vector and keyword search results
// Applies weighted scoring: score = vectorWeight * vectorScore + textWeight * textScore
func MergeHybridResults(vector []HybridVectorResult, keyword []HybridKeywordResult, vectorWeight, textWeight float64) []*SearchResult {
	// Build ID -> item map for merged results
	byID := make(map[string]*hybridItem)

	// Add vector results
	for _, v := range vector {
		byID[v.ID] = &hybridItem{
			ID:          v.ID,
			Path:        v.Path,
			StartLine:   v.StartLine,
			EndLine:     v.EndLine,
			Source:      v.Source,
			Snippet:     v.Snippet,
			VectorScore: v.VectorScore,
			TextScore:   0,
		}
	}

	// Merge keyword results
	for _, k := range keyword {
		if existing, ok := byID[k.ID]; ok {
			existing.TextScore = k.TextScore
			if k.Snippet != "" {
				existing.Snippet = k.Snippet
			}
		} else {
			byID[k.ID] = &hybridItem{
				ID:          k.ID,
				Path:        k.Path,
				StartLine:   k.StartLine,
				EndLine:     k.EndLine,
				Source:      k.Source,
				Snippet:     k.Snippet,
				VectorScore: 0,
				TextScore:   k.TextScore,
			}
		}
	}

	// Convert to search results with weighted scores
	results := make([]*SearchResult, 0, len(byID))
	for _, item := range byID {
		score := vectorWeight*item.VectorScore + textWeight*item.TextScore

		// Determine source type
		var source MemorySource
		switch item.Source {
		case "longterm", "MEMORY.md", "memory.md":
			source = MemorySourceLongTerm
		case "daily":
			source = MemorySourceDaily
		case "session":
			source = MemorySourceSession
		default:
			// Try to infer from path
			if strings.Contains(item.Path, "memory/") && strings.Contains(item.Path, "-") {
				source = MemorySourceDaily
			} else if strings.Contains(item.Path, "MEMORY") || strings.Contains(item.Path, "memory.md") {
				source = MemorySourceLongTerm
			} else {
				source = MemorySourceLongTerm
			}
		}

		results = append(results, &SearchResult{
			VectorEmbedding: VectorEmbedding{
				ID:     item.ID,
				Text:   item.Snippet,
				Source: source,
				Metadata: MemoryMetadata{
					FilePath:      item.Path,
					LineNumber:    item.StartLine,
					EndLineNumber: item.EndLine,
				},
			},
			Score:       score,
			VectorScore: item.VectorScore,
			TextScore:   item.TextScore,
		})
	}

	return results
}

// ExtractKeywords extracts important keywords from query for expansion
func ExtractKeywords(query string) []string {
	// Remove common stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "was": true, "are": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "can": true,
	}

	words := strings.Fields(query)
	var keywords []string

	for _, word := range words {
		// Clean word
		clean := strings.Trim(strings.ToLower(word), ".,!?;:'\"()[]{}")
		if len(clean) > 2 && !stopWords[clean] {
			keywords = append(keywords, clean)
		}
	}

	return keywords
}

// NormalizeScores normalizes scores to [0, 1] range
func NormalizeScores(results []*SearchResult) []*SearchResult {
	if len(results) == 0 {
		return results
	}

	// Find min and max
	minScore := results[0].Score
	maxScore := results[0].Score
	for _, r := range results {
		if r.Score < minScore {
			minScore = r.Score
		}
		if r.Score > maxScore {
			maxScore = r.Score
		}
	}

	// Normalize
	if maxScore > minScore {
		for _, r := range results {
			r.Score = (r.Score - minScore) / (maxScore - minScore)
		}
	} else {
		// All same score, set to 1
		for _, r := range results {
			r.Score = 1.0
		}
	}

	return results
}
