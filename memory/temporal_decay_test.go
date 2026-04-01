package memory

import (
	"math"
	"testing"
	"time"
)

func TestToDecayLambda(t *testing.T) {
	tests := []struct {
		name         string
		halfLifeDays float64
		expected     float64
	}{
		{
			name:         "30 day half-life",
			halfLifeDays: 30,
			expected:     math.Ln2 / 30,
		},
		{
			name:         "1 day half-life",
			halfLifeDays: 1,
			expected:     math.Ln2,
		},
		{
			name:         "zero half-life",
			halfLifeDays: 0,
			expected:     0,
		},
		{
			name:         "negative half-life",
			halfLifeDays: -5,
			expected:     0,
		},
		{
			name:         "infinite half-life",
			halfLifeDays: math.Inf(1),
			expected:     0,
		},
		{
			name:         "NaN half-life",
			halfLifeDays: math.NaN(),
			expected:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toDecayLambda(tt.halfLifeDays)
			if math.IsNaN(tt.expected) {
				if !math.IsNaN(result) {
					t.Errorf("expected NaN, got %v", result)
				}
			} else if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("toDecayLambda() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCalculateTemporalDecayMultiplier(t *testing.T) {
	tests := []struct {
		name         string
		ageInDays    float64
		halfLifeDays float64
		expected     float64
	}{
		{
			name:         "zero age",
			ageInDays:    0,
			halfLifeDays: 30,
			expected:     1.0,
		},
		{
			name:         "one half-life",
			ageInDays:    30,
			halfLifeDays: 30,
			expected:     0.5,
		},
		{
			name:         "two half-lives",
			ageInDays:    60,
			halfLifeDays: 30,
			expected:     0.25,
		},
		{
			name:         "negative age (clamped to 0)",
			ageInDays:    -10,
			halfLifeDays: 30,
			expected:     1.0,
		},
		{
			name:         "zero half-life",
			ageInDays:    10,
			halfLifeDays: 0,
			expected:     1.0,
		},
		{
			name:         "infinite age",
			ageInDays:    math.Inf(1),
			halfLifeDays: 30,
			expected:     1.0,
		},
		{
			name:         "NaN age",
			ageInDays:    math.NaN(),
			halfLifeDays: 30,
			expected:     1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateTemporalDecayMultiplier(tt.ageInDays, tt.halfLifeDays)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("calculateTemporalDecayMultiplier() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestApplyTemporalDecayToScore(t *testing.T) {
	tests := []struct {
		name         string
		score        float64
		ageInDays    float64
		halfLifeDays float64
		expected     float64
	}{
		{
			name:         "fresh memory",
			score:        1.0,
			ageInDays:    0,
			halfLifeDays: 30,
			expected:     1.0,
		},
		{
			name:         "one half-life",
			score:        1.0,
			ageInDays:    30,
			halfLifeDays: 30,
			expected:     0.5,
		},
		{
			name:         "high score old memory",
			score:        0.9,
			ageInDays:    60,
			halfLifeDays: 30,
			expected:     0.225,
		},
		{
			name:         "low score recent memory",
			score:        0.3,
			ageInDays:    5,
			halfLifeDays: 30,
			expected:     0.3 * calculateTemporalDecayMultiplier(5, 30),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyTemporalDecayToScore(tt.score, tt.ageInDays, tt.halfLifeDays)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("applyTemporalDecayToScore() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseMemoryDateFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected *time.Time
	}{
		{
			name: "valid daily memory path",
			path: "memory/2024-02-15.md",
			expected: func() *time.Time {
				tm := time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC)
				return &tm
			}(),
		},
		{
			name: "valid daily memory path with subdirectory",
			path: "project/memory/2024-12-31.md",
			expected: func() *time.Time {
				tm := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
				return &tm
			}(),
		},
		{
			name:     "MEMORY.md file",
			path:     "MEMORY.md",
			expected: nil,
		},
		{
			name:     "invalid date format",
			path:     "memory/2024-2-15.md",
			expected: nil,
		},
		{
			name:     "non-memory file",
			path:     "other/2024-02-15.txt",
			expected: nil,
		},
		{
			name:     "empty path",
			path:     "",
			expected: nil,
		},
		{
			name: "windows-style path",
			path: "memory/2024-02-15.md",
			expected: func() *time.Time {
				tm := time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC)
				return &tm
			}(),
		},
		{
			name:     "invalid year",
			path:     "memory/abcd-02-15.md",
			expected: nil,
		},
		{
			name:     "invalid month",
			path:     "memory/2024-ab-15.md",
			expected: nil,
		},
		{
			name:     "invalid day",
			path:     "memory/2024-02-ab.md",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMemoryDateFromPath(tt.path)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("expected %v, got nil", tt.expected)
				} else if !result.Equal(*tt.expected) {
					t.Errorf("parseMemoryDateFromPath() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestIsEvergreenMemoryPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "MEMORY.md at root",
			path:     "MEMORY.md",
			expected: true,
		},
		{
			name:     "memory.md lowercase",
			path:     "memory.md",
			expected: true,
		},
		{
			name:     "topic file in memory root",
			path:     "memory/topics.md",
			expected: true,
		},
		{
			name:     "daily memory file",
			path:     "memory/2024-02-15.md",
			expected: false,
		},
		{
			name:     "daily memory in subdirectory",
			path:     "project/memory/2024-02-15.md",
			expected: false,
		},
		{
			name:     "config file",
			path:     "config.json",
			expected: false,
		},
		{
			name:     "windows path MEMORY.md",
			path:     "MEMORY.md",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEvergreenMemoryPath(tt.path)
			if result != tt.expected {
				t.Errorf("isEvergreenMemoryPath() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractTimestamp(t *testing.T) {
	now := time.Now()
	pastTime := now.Add(-24 * time.Hour)

	tests := []struct {
		name     string
		ve       *VectorEmbedding
		expected bool
	}{
		{
			name: "metadata timestamp present",
			ve: &VectorEmbedding{
				Metadata: MemoryMetadata{
					Timestamp: &pastTime,
				},
			},
			expected: true,
		},
		{
			name: "daily memory path",
			ve: &VectorEmbedding{
				Source: MemorySourceDaily,
				Metadata: MemoryMetadata{
					FilePath: "memory/2024-02-15.md",
				},
			},
			expected: true,
		},
		{
			name: "evergreen memory - MEMORY.md",
			ve: &VectorEmbedding{
				Source: MemorySourceLongTerm,
				Metadata: MemoryMetadata{
					FilePath: "MEMORY.md",
				},
			},
			expected: false,
		},
		{
			name: "evergreen memory - topic file",
			ve: &VectorEmbedding{
				Source: MemorySourceLongTerm,
				Metadata: MemoryMetadata{
					FilePath: "memory/topics.md",
				},
			},
			expected: false,
		},
		{
			name: "fallback to created_at",
			ve: &VectorEmbedding{
				CreatedAt: now,
				Metadata:  MemoryMetadata{},
			},
			expected: true,
		},
		{
			name: "session memory with timestamp",
			ve: &VectorEmbedding{
				Source: MemorySourceSession,
				Metadata: MemoryMetadata{
					Timestamp: &pastTime,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTimestamp(tt.ve)
			if tt.expected {
				if result == nil {
					t.Errorf("expected non-nil timestamp")
				}
			} else {
				if result != nil {
					t.Errorf("expected nil timestamp, got %v", result)
				}
			}
		})
	}
}

func TestApplyTemporalDecayToResults(t *testing.T) {
	now := time.Date(2024, 2, 20, 0, 0, 0, 0, time.UTC)
	past10Days := now.Add(-10 * 24 * time.Hour)
	past30Days := now.Add(-30 * 24 * time.Hour)

	tests := []struct {
		name           string
		results        []*SearchResult
		config         TemporalDecayConfig
		expectedOrder  []string
		expectedScores []float64
	}{
		{
			name:           "empty results",
			results:        []*SearchResult{},
			config:         TemporalDecayConfig{Enabled: true, HalfLifeDays: 30},
			expectedOrder:  []string{},
			expectedScores: []float64{},
		},
		{
			name: "disabled temporal decay",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{ID: "r1"},
					Score:           0.9,
				},
			},
			config:         TemporalDecayConfig{Enabled: false},
			expectedOrder:  []string{"r1"},
			expectedScores: []float64{0.9},
		},
		{
			name: "single result with decay",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID: "r1",
						Metadata: MemoryMetadata{
							Timestamp: &past10Days,
						},
					},
					Score: 1.0,
				},
			},
			config:         TemporalDecayConfig{Enabled: true, HalfLifeDays: 30},
			expectedOrder:  []string{"r1"},
			expectedScores: []float64{calculateTemporalDecayMultiplier(10, 30)},
		},
		{
			name: "multiple results reordered by decay",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID: "old-high-score",
						Metadata: MemoryMetadata{
							Timestamp: &past30Days,
						},
					},
					Score: 1.0,
				},
				{
					VectorEmbedding: VectorEmbedding{
						ID: "recent-low-score",
						Metadata: MemoryMetadata{
							Timestamp: &past10Days,
						},
					},
					Score: 0.8,
				},
			},
			config:         TemporalDecayConfig{Enabled: true, HalfLifeDays: 30},
			expectedOrder:  []string{"recent-low-score", "old-high-score"},
			expectedScores: nil,
		},
		{
			name: "evergreen memory no decay",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID:     "evergreen",
						Source: MemorySourceLongTerm,
						Metadata: MemoryMetadata{
							FilePath: "MEMORY.md",
						},
					},
					Score: 1.0,
				},
			},
			config:         TemporalDecayConfig{Enabled: true, HalfLifeDays: 30},
			expectedOrder:  []string{"evergreen"},
			expectedScores: []float64{1.0},
		},
		{
			name: "zero half-life defaults to 30",
			results: []*SearchResult{
				{
					VectorEmbedding: VectorEmbedding{
						ID: "r1",
						Metadata: MemoryMetadata{
							Timestamp: &past30Days,
						},
					},
					Score: 1.0,
				},
			},
			config:         TemporalDecayConfig{Enabled: true, HalfLifeDays: 0},
			expectedOrder:  []string{"r1"},
			expectedScores: []float64{0.5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyTemporalDecayToResults(tt.results, tt.config, now)

			if len(result) != len(tt.expectedOrder) {
				t.Errorf("expected %d results, got %d", len(tt.expectedOrder), len(result))
				return
			}

			for i, r := range result {
				if r.ID != tt.expectedOrder[i] {
					t.Errorf("position %d: expected ID %s, got %s", i, tt.expectedOrder[i], r.ID)
				}
				if tt.expectedScores != nil && i < len(tt.expectedScores) {
					if math.Abs(r.Score-tt.expectedScores[i]) > 0.01 {
						t.Errorf("position %d: expected score %v, got %v", i, tt.expectedScores[i], r.Score)
					}
				}
			}
		})
	}
}

func TestApplyTemporalDecay(t *testing.T) {
	results := []*SearchResult{
		{
			VectorEmbedding: VectorEmbedding{
				ID: "r1",
				Metadata: MemoryMetadata{
					Timestamp: ptrTime(time.Now().Add(-24 * time.Hour)),
				},
			},
			Score: 1.0,
		},
	}

	config := TemporalDecayConfig{Enabled: true, HalfLifeDays: 30}
	decayed := ApplyTemporalDecay(results, config)

	if len(decayed) != 1 {
		t.Errorf("expected 1 result, got %d", len(decayed))
	}

	if decayed[0].Score >= 1.0 {
		t.Errorf("score should decay for 1-day old memory, got %v", decayed[0].Score)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
