package memory

import (
	"math"
)

// mmrItem represents an item for MMR processing
type mmrItem struct {
	id        string
	score     float64
	content   string
	vector    []float32
	embedding *VectorEmbedding
}

// cosineSimilarity computes cosine similarity between two vectors
// Returns a value in [-1, 1] where 1 means identical direction
func cosineSimilarity(vecA, vecB []float32) float64 {
	if len(vecA) == 0 || len(vecB) == 0 || len(vecA) != len(vecB) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range vecA {
		dotProduct += float64(vecA[i]) * float64(vecB[i])
		normA += float64(vecA[i]) * float64(vecA[i])
		normB += float64(vecB[i]) * float64(vecB[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// maxSimilarityToSelected computes the maximum similarity between an item and all selected items
func maxSimilarityToSelected(item mmrItem, selectedItems []mmrItem) float64 {
	if len(selectedItems) == 0 {
		return 0
	}

	maxSim := 0.0
	for _, selected := range selectedItems {
		if len(item.vector) > 0 && len(selected.vector) > 0 {
			sim := cosineSimilarity(item.vector, selected.vector)
			if sim > maxSim {
				maxSim = sim
			}
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

	// Convert to MMR items with embeddings
	items := make([]mmrItem, len(results))
	for i, r := range results {
		items[i] = mmrItem{
			id:        r.ID,
			score:     r.Score,
			content:   r.Text,
			vector:    r.Vector,
			embedding: &r.VectorEmbedding,
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
