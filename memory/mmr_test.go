package memory

import (
	"math"
	"testing"
)

func TestCosineSimilarity_MMR(t *testing.T) {
	tests := []struct {
		name     string
		vecA     []float32
		vecB     []float32
		expected float64
	}{
		{
			name:     "identical vectors",
			vecA:     []float32{1.0, 2.0, 3.0},
			vecB:     []float32{1.0, 2.0, 3.0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			vecA:     []float32{1.0, 0.0},
			vecB:     []float32{0.0, 1.0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			vecA:     []float32{1.0, 0.0},
			vecB:     []float32{-1.0, 0.0},
			expected: -1.0,
		},
		{
			name:     "parallel vectors",
			vecA:     []float32{1.0, 2.0},
			vecB:     []float32{2.0, 4.0},
			expected: 1.0,
		},
		{
			name:     "zero vector A",
			vecA:     []float32{0.0, 0.0, 0.0},
			vecB:     []float32{1.0, 2.0, 3.0},
			expected: 0.0,
		},
		{
			name:     "zero vector B",
			vecA:     []float32{1.0, 2.0, 3.0},
			vecB:     []float32{0.0, 0.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "both zero vectors",
			vecA:     []float32{0.0, 0.0},
			vecB:     []float32{0.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "empty vector A",
			vecA:     []float32{},
			vecB:     []float32{1.0, 2.0},
			expected: 0.0,
		},
		{
			name:     "empty vector B",
			vecA:     []float32{1.0, 2.0},
			vecB:     []float32{},
			expected: 0.0,
		},
		{
			name:     "different dimensions",
			vecA:     []float32{1.0, 2.0, 3.0},
			vecB:     []float32{1.0, 2.0},
			expected: 0.0,
		},
		{
			name:     "45 degree angle",
			vecA:     []float32{1.0, 0.0},
			vecB:     []float32{1.0, 1.0},
			expected: 0.7071067811865476,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.vecA, tt.vecB)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("cosineSimilarity() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMaxSimilarityToSelected(t *testing.T) {
	tests := []struct {
		name           string
		item           mmrItem
		selected       []mmrItem
		expectedMaxSim float64
	}{
		{
			name: "no selected items",
			item: mmrItem{
				id:     "item1",
				vector: []float32{1.0, 0.0},
			},
			selected:       []mmrItem{},
			expectedMaxSim: 0.0,
		},
		{
			name: "single selected item - identical",
			item: mmrItem{
				id:     "item1",
				vector: []float32{1.0, 0.0},
			},
			selected: []mmrItem{
				{id: "selected1", vector: []float32{1.0, 0.0}},
			},
			expectedMaxSim: 1.0,
		},
		{
			name: "single selected item - orthogonal",
			item: mmrItem{
				id:     "item1",
				vector: []float32{1.0, 0.0},
			},
			selected: []mmrItem{
				{id: "selected1", vector: []float32{0.0, 1.0}},
			},
			expectedMaxSim: 0.0,
		},
		{
			name: "multiple selected items - max similarity",
			item: mmrItem{
				id:     "item1",
				vector: []float32{1.0, 0.0},
			},
			selected: []mmrItem{
				{id: "s1", vector: []float32{0.0, 1.0}},
				{id: "s2", vector: []float32{0.8, 0.6}},
				{id: "s3", vector: []float32{-1.0, 0.0}},
			},
			expectedMaxSim: 0.8,
		},
		{
			name: "item without vector",
			item: mmrItem{
				id:     "item1",
				vector: nil,
			},
			selected: []mmrItem{
				{id: "s1", vector: []float32{1.0, 0.0}},
			},
			expectedMaxSim: 0.0,
		},
		{
			name: "selected item without vector",
			item: mmrItem{
				id:     "item1",
				vector: []float32{1.0, 0.0},
			},
			selected: []mmrItem{
				{id: "s1", vector: nil},
			},
			expectedMaxSim: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maxSimilarityToSelected(tt.item, tt.selected)
			if math.Abs(result-tt.expectedMaxSim) > 0.0001 {
				t.Errorf("maxSimilarityToSelected() = %v, want %v", result, tt.expectedMaxSim)
			}
		})
	}
}

func TestComputeMMRScore(t *testing.T) {
	tests := []struct {
		name        string
		relevance   float64
		maxSim      float64
		lambda      float64
		expectedMMR float64
	}{
		{
			name:        "lambda 1 - only relevance",
			relevance:   0.9,
			maxSim:      0.8,
			lambda:      1.0,
			expectedMMR: 0.9,
		},
		{
			name:        "lambda 0 - only diversity",
			relevance:   0.9,
			maxSim:      0.8,
			lambda:      0.0,
			expectedMMR: -0.8,
		},
		{
			name:        "lambda 0.5 - balanced",
			relevance:   1.0,
			maxSim:      0.6,
			lambda:      0.5,
			expectedMMR: 0.2,
		},
		{
			name:        "no similarity penalty",
			relevance:   0.8,
			maxSim:      0.0,
			lambda:      0.7,
			expectedMMR: 0.56,
		},
		{
			name:        "high similarity penalty",
			relevance:   0.7,
			maxSim:      1.0,
			lambda:      0.5,
			expectedMMR: -0.15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeMMRScore(tt.relevance, tt.maxSim, tt.lambda)
			if math.Abs(result-tt.expectedMMR) > 0.0001 {
				t.Errorf("computeMMRScore() = %v, want %v", result, tt.expectedMMR)
			}
		})
	}
}

func TestApplyMMR(t *testing.T) {
	tests := []struct {
		name          string
		results       []*SearchResult
		lambda        float64
		expectedOrder []string
	}{
		{
			name:          "empty results",
			results:       []*SearchResult{},
			lambda:        0.7,
			expectedOrder: []string{},
		},
		{
			name: "single result",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r1",
						Vector: []float32{1.0, 0.0},
					},
					Score: 0.9,
				},
			},
			lambda:        0.7,
			expectedOrder: []string{"r1"},
		},
		{
			name: "identical vectors - order by score",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r1",
						Vector: []float32{1.0, 0.0},
					},
					Score: 0.9,
				},
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r2",
						Vector: []float32{1.0, 0.0},
					},
					Score: 0.8,
				},
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r3",
						Vector: []float32{1.0, 0.0},
					},
					Score: 0.7,
				},
			},
			lambda:        0.7,
			expectedOrder: []string{"r1", "r2", "r3"},
		},
		{
			name: "diverse vectors - lambda 1 (no diversity)",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r1",
						Vector: []float32{1.0, 0.0},
					},
					Score: 0.9,
				},
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r2",
						Vector: []float32{0.0, 1.0},
					},
					Score: 0.8,
				},
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r3",
						Vector: []float32{-1.0, 0.0},
					},
					Score: 0.7,
				},
			},
			lambda:        1.0,
			expectedOrder: []string{"r1", "r2", "r3"},
		},
		{
			name: "similar vectors - lambda 0.5 (balanced)",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "high-sim",
						Vector: []float32{0.9, 0.1},
					},
					Score: 0.85,
				},
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "low-sim",
						Vector: []float32{0.0, 1.0},
					},
					Score: 0.8,
				},
			},
			lambda:        0.5,
			expectedOrder: []string{"high-sim", "low-sim"},
		},
		{
			name: "lambda out of bounds - clamped to 1",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r1",
						Vector: []float32{1.0, 0.0},
					},
					Score: 0.9,
				},
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r2",
						Vector: []float32{0.0, 1.0},
					},
					Score: 0.8,
				},
			},
			lambda:        1.5,
			expectedOrder: []string{"r1", "r2"},
		},
		{
			name: "lambda out of bounds - clamped to 0",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r1",
						Vector: []float32{1.0, 0.0},
					},
					Score: 0.9,
				},
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "r2",
						Vector: []float32{0.99, 0.01},
					},
					Score: 0.8,
				},
			},
			lambda:        -0.5,
			expectedOrder: []string{"r1", "r2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyMMR(tt.results, tt.lambda)
			if len(result) != len(tt.expectedOrder) {
				t.Errorf("expected %d results, got %d", len(tt.expectedOrder), len(result))
				return
			}
			for i, r := range result {
				if r.ID != tt.expectedOrder[i] {
					t.Errorf("position %d: expected ID %s, got %s", i, tt.expectedOrder[i], r.ID)
				}
			}
		})
	}
}

func TestApplyMMR_Diversity(t *testing.T) {
	results := []*SearchResult{
		{
			VectorEmbedding: VectorEmbedding{
				ID:     "high-rel-high-sim",
				Vector: []float32{1.0, 0.0, 0.0},
			},
			Score: 0.95,
		},
		{
			VectorEmbedding: VectorEmbedding{
				ID:     "high-rel-sim",
				Vector: []float32{0.95, 0.1, 0.0},
			},
			Score: 0.9,
		},
		{
			VectorEmbedding: VectorEmbedding{
				ID:     "med-rel-diverse",
				Vector: []float32{0.0, 1.0, 0.0},
			},
			Score: 0.8,
		},
		{
			VectorEmbedding: VectorEmbedding{
				ID:     "low-rel-diverse",
				Vector: []float32{0.0, 0.0, 1.0},
			},
			Score: 0.7,
		},
	}

	reordered := applyMMR(results, 0.7)

	if len(reordered) != 4 {
		t.Errorf("expected 4 results, got %d", len(reordered))
		return
	}

	if reordered[0].ID != "high-rel-high-sim" {
		t.Errorf("first result should be highest relevance, got %s", reordered[0].ID)
	}

	if reordered[1].ID != "med-rel-diverse" {
		t.Errorf("second result should be diverse, got %s", reordered[1].ID)
	}
}

func TestApplyMMR_Public(t *testing.T) {
	tests := []struct {
		name        string
		results     []*SearchResult
		config      MMRConfig
		expectedLen int
	}{
		{
			name: "disabled MMR",
			results: []*SearchResult{
				{VectorEmbedding: VectorEmbedding{ID: "r1"}},
				{VectorEmbedding: VectorEmbedding{ID: "r2"}},
			},
			config:      MMRConfig{Enabled: false, Lambda: 0.7},
			expectedLen: 2,
		},
		{
			name:        "single result",
			results:     []*SearchResult{{VectorEmbedding: VectorEmbedding{ID: "r1"}}},
			config:      MMRConfig{Enabled: true, Lambda: 0.7},
			expectedLen: 1,
		},
		{
			name: "enabled with multiple results",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{ID: "r1", Vector: []float32{1, 0}},
					Score:           0.9,
				},
				{
					VectorEmbedding: VectorEmbedding{ID: "r2", Vector: []float32{0, 1}},
					Score:           0.8,
				},
			},
			config:      MMRConfig{Enabled: true, Lambda: 0.7},
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyMMR(tt.results, tt.config)
			if len(result) != tt.expectedLen {
				t.Errorf("expected %d results, got %d", tt.expectedLen, len(result))
			}
		})
	}
}

func TestApplyMMR_PreservesResults(t *testing.T) {
	results := []*SearchResult{
		{
			VectorEmbedding: VectorEmbedding{
				ID:     "r1",
				Text:   "test text",
				Vector: []float32{1.0, 0.0},
			},
			Score: 0.9,
		},
	}

	reordered := applyMMR(results, 0.7)

	if len(reordered) != 1 {
		t.Errorf("expected 1 result, got %d", len(reordered))
		return
	}

	if reordered[0].Text != "test text" {
		t.Errorf("text not preserved")
	}
	if reordered[0].VectorEmbedding.ID != "r1" {
		t.Errorf("embedding not preserved")
	}
}
