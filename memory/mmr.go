package memory

import (
	"math"
	"regexp"
	"strings"
)

// mmrItem represents an item for MMR processing
type mmrItem struct {
	id      string
	score   float64
	content string
	tokens  map[string]struct{}
}

// tokenize extracts alphanumeric tokens from text and normalizes to lowercase
func tokenize(text string) map[string]struct{} {
	re := regexp.MustCompile(`[a-z0-9_]+`)
	matches := re.FindAllString(strings.ToLower(text), -1)
	tokens := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		tokens[m] = struct{}{}
	}
	return tokens
}

// jaccardSimilarity computes Jaccard similarity between two token sets
// Returns a value in [0, 1] where 1 means identical sets
func jaccardSimilarity(setA, setB map[string]struct{}) float64 {
	if len(setA) == 0 && len(setB) == 0 {
		return 1
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}

	// Find intersection size
	intersection := 0
	for token := range setA {
		if _, exists := setB[token]; exists {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// maxSimilarityToSelected computes the maximum similarity between an item and all selected items
func maxSimilarityToSelected(item mmrItem, selectedItems []mmrItem) float64 {
	if len(selectedItems) == 0 {
		return 0
	}

	maxSim := 0.0
	for _, selected := range selectedItems {
		sim := jaccardSimilarity(item.tokens, selected.tokens)
		if sim > maxSim {
			maxSim = sim
		}
	}
	return maxSim
}

// computeMMRScore computes MMR score for a candidate item
// MMR = λ * relevance - (1-λ) * max_similarity_to_selected
func computeMMRScore(relevance, maxSimilarity, lambda float64) float64 {
	return lambda*relevance - (1-lambda)*maxSimilarity
}

// applyMMR re-ranks search results using Maximal Marginal Relevance
// MMR balances relevance with diversity by iteratively selecting results
// that maximize: λ * relevance - (1-λ) * max_similarity_to_selected
func applyMMR(results []*SearchResult, lambda float64) []*SearchResult {
	if len(results) == 0 {
		return results
	}

	// Clamp lambda to [0, 1]
	lambda = math.Max(0, math.Min(1, lambda))

	// Convert to MMR items
	items := make([]mmrItem, len(results))
	for i, r := range results {
		items[i] = mmrItem{
			id:      r.ID,
			score:   r.Score,
			content: r.Text,
			tokens:  tokenize(r.Text),
		}
	}

	var selected []mmrItem
	var remaining []mmrItem
	remaining = append(remaining, items...)

	// Greedy selection: pick the item with highest MMR score iteratively
	for len(remaining) > 0 {
		bestIdx := -1
		bestScore := math.Inf(-1)

		for i, item := range remaining {
			maxSim := maxSimilarityToSelected(item, selected)
			mmrScore := computeMMRScore(item.score, maxSim, lambda)
			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}

		if bestIdx < 0 {
			break
		}

		// Move best item from remaining to selected
		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	// Rebuild results in MMR order
	resultMap := make(map[string]*SearchResult, len(results))
	for _, r := range results {
		resultMap[r.ID] = r
	}

	reordered := make([]*SearchResult, 0, len(selected))
	for _, item := range selected {
		if r, exists := resultMap[item.id]; exists {
			// Update score to reflect MMR ranking
			r.Score = item.score
			reordered = append(reordered, r)
		}
	}

	return reordered
}

// ApplyMMR re-ranks search results using MMR configuration
func ApplyMMR(results []*SearchResult, config MMRConfig) []*SearchResult {
	if !config.Enabled || len(results) < 2 {
		return results
	}
	return applyMMR(results, config.Lambda)
}
