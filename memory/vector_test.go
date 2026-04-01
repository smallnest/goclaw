package memory

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		wantErr  bool
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0},
			b:        []float32{0, 1},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 1, 1},
			b:        []float32{-1, -1, -1},
			expected: -1.0,
		},
		{
			name:     "similar vectors",
			a:        []float32{3, 4},
			b:        []float32{6, 8},
			expected: 1.0,
		},
		{
			name:     "dimension mismatch",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2},
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{1, 2},
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CosineSimilarity(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if math.Abs(result-tt.expected) > 0.001 {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			}
		})
	}
}

func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		wantErr  bool
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "3-4-5 triangle",
			a:        []float32{0, 0},
			b:        []float32{3, 4},
			expected: 5.0,
		},
		{
			name:     "simple distance",
			a:        []float32{1, 1},
			b:        []float32{4, 5},
			expected: 5.0,
		},
		{
			name:     "dimension mismatch",
			a:        []float32{1, 2},
			b:        []float32{1},
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{1, 2},
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EuclideanDistance(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if math.Abs(result-tt.expected) > 0.001 {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			}
		})
	}
}

func TestDotProduct(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		wantErr  bool
	}{
		{
			name:     "simple dot product",
			a:        []float32{1, 2, 3},
			b:        []float32{4, 5, 6},
			expected: 32.0, // 1*4 + 2*5 + 3*6 = 32
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0},
			b:        []float32{0, 1},
			expected: 0.0,
		},
		{
			name:     "negative values",
			a:        []float32{-1, 2},
			b:        []float32{3, -4},
			expected: -11.0,
		},
		{
			name:     "dimension mismatch",
			a:        []float32{1, 2},
			b:        []float32{1},
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DotProduct(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if math.Abs(result-tt.expected) > 0.001 {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		vec     []float32
		wantErr bool
	}{
		{
			name: "normalize 3-4 vector",
			vec:  []float32{3, 4},
		},
		{
			name: "normalize simple vector",
			vec:  []float32{1, 1, 1},
		},
		{
			name:    "empty vector",
			vec:     []float32{},
			wantErr: true,
		},
		{
			name:    "zero vector",
			vec:     []float32{0, 0, 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.vec)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				mag, _ := Magnitude(result)
				if math.Abs(mag-1.0) > 0.001 {
					t.Errorf("Normalized vector should have magnitude 1.0, got %v", mag)
				}
			}
		})
	}
}

func TestMagnitude(t *testing.T) {
	tests := []struct {
		name     string
		vec      []float32
		expected float64
		wantErr  bool
	}{
		{
			name:     "3-4-5 triangle",
			vec:      []float32{3, 4},
			expected: 5.0,
		},
		{
			name:     "unit vector",
			vec:      []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "zero vector",
			vec:      []float32{0, 0},
			expected: 0.0,
		},
		{
			name:     "empty vector",
			vec:      []float32{},
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Magnitude(tt.vec)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if math.Abs(result-tt.expected) > 0.001 {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			}
		})
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name    string
		a       []float32
		b       []float32
		wantErr bool
	}{
		{
			name: "simple addition",
			a:    []float32{1, 2, 3},
			b:    []float32{4, 5, 6},
		},
		{
			name: "negative values",
			a:    []float32{-1, 2},
			b:    []float32{3, -4},
		},
		{
			name:    "dimension mismatch",
			a:       []float32{1, 2},
			b:       []float32{1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Add(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(result) != len(tt.a) {
					t.Errorf("Expected length %d, got %d", len(tt.a), len(result))
				}
				for i := range result {
					expected := tt.a[i] + tt.b[i]
					if result[i] != expected {
						t.Errorf("At index %d: expected %v, got %v", i, expected, result[i])
					}
				}
			}
		})
	}
}

func TestSubtract(t *testing.T) {
	tests := []struct {
		name    string
		a       []float32
		b       []float32
		wantErr bool
	}{
		{
			name: "simple subtraction",
			a:    []float32{5, 6, 7},
			b:    []float32{1, 2, 3},
		},
		{
			name:    "dimension mismatch",
			a:       []float32{1, 2},
			b:       []float32{1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Subtract(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				for i := range result {
					expected := tt.a[i] - tt.b[i]
					if result[i] != expected {
						t.Errorf("At index %d: expected %v, got %v", i, expected, result[i])
					}
				}
			}
		})
	}
}

func TestMultiply(t *testing.T) {
	tests := []struct {
		name    string
		vec     []float32
		scalar  float64
		wantErr bool
	}{
		{
			name:   "multiply by 2",
			vec:    []float32{1, 2, 3},
			scalar: 2.0,
		},
		{
			name:   "multiply by 0.5",
			vec:    []float32{4, 6},
			scalar: 0.5,
		},
		{
			name:   "multiply by negative",
			vec:    []float32{1, 2},
			scalar: -1.0,
		},
		{
			name:    "empty vector",
			vec:     []float32{},
			scalar:  2.0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Multiply(tt.vec, tt.scalar)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				for i := range result {
					expected := float32(float64(tt.vec[i]) * tt.scalar)
					if result[i] != expected {
						t.Errorf("At index %d: expected %v, got %v", i, expected, result[i])
					}
				}
			}
		})
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		name    string
		vecs    [][]float32
		wantErr bool
	}{
		{
			name: "simple mean",
			vecs: [][]float32{
				{1, 2},
				{3, 4},
				{5, 6},
			},
		},
		{
			name:    "no vectors",
			vecs:    [][]float32{},
			wantErr: true,
		},
		{
			name:    "dimension mismatch",
			vecs:    [][]float32{{1, 2}, {1}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Mean(tt.vecs)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				expectedDim := len(tt.vecs[0])
				if len(result) != expectedDim {
					t.Errorf("Expected length %d, got %d", expectedDim, len(result))
				}
				// First test case: (1+3+5)/3 = 3, (2+4+6)/3 = 4
				if expectedDim == 2 {
					if math.Abs(float64(result[0])-3.0) > 0.001 {
						t.Errorf("Expected 3.0, got %v", result[0])
					}
					if math.Abs(float64(result[1])-4.0) > 0.001 {
						t.Errorf("Expected 4.0, got %v", result[1])
					}
				}
			}
		})
	}
}

func TestVectorChunkText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxTokens int
		minChunks int
		maxChunks int
	}{
		{
			name:      "small text",
			text:      "small text",
			maxTokens: 100,
			minChunks: 1,
			maxChunks: 1,
		},
		{
			name:      "large text",
			text:      "This is a sentence. This is another sentence. And a third one. " + "This is a sentence. This is another sentence. And a third one. " + "This is a sentence. This is another sentence. And a third one. " + "This is a sentence. This is another sentence. And a third one. " + "This is a sentence. This is another sentence. And a third one. ",
			maxTokens: 50,
			minChunks: 2,
			maxChunks: 10,
		},
		{
			name:      "single long word",
			text:      "a" + string(make([]byte, 200)) + "b",
			maxTokens: 50,
			minChunks: 2,
			maxChunks: 10,
		},
		{
			name:      "empty text",
			text:      "",
			maxTokens: 100,
			minChunks: 1,
			maxChunks: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text, tt.maxTokens)
			if len(chunks) < tt.minChunks || len(chunks) > tt.maxChunks {
				t.Errorf("Expected %d-%d chunks, got %d", tt.minChunks, tt.maxChunks, len(chunks))
			}
		})
	}
}

func TestComputeHash(t *testing.T) {
	vec1 := []float32{1, 2, 3}
	vec2 := []float32{1, 2, 3}
	vec3 := []float32{1, 2, 4}

	hash1 := ComputeHash(vec1)
	hash2 := ComputeHash(vec2)
	hash3 := ComputeHash(vec3)

	if hash1 != hash2 {
		t.Error("Expected same hash for identical vectors")
	}

	if hash1 == hash3 {
		t.Error("Expected different hashes for different vectors")
	}
}

func TestComputeHash_EmptyVector(t *testing.T) {
	vec := []float32{}
	hash := ComputeHash(vec)

	if hash == 0 {
		t.Error("Expected non-zero hash for empty vector")
	}
}

func TestCosineSimilarity_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		a       []float32
		b       []float32
		wantErr bool
	}{
		{
			name:    "very small values",
			a:       []float32{1e-10, 1e-10},
			b:       []float32{1e-10, 1e-10},
			wantErr: false,
		},
		{
			name:    "very large values",
			a:       []float32{1e10, 1e10},
			b:       []float32{1e10, 1e10},
			wantErr: false,
		},
		{
			name:    "mixed positive negative",
			a:       []float32{-1, 2, -3, 4},
			b:       []float32{1, -2, 3, -4},
			wantErr: false,
		},
		{
			name:    "single dimension",
			a:       []float32{1.5},
			b:       []float32{2.5},
			wantErr: false,
		},
		{
			name:    "high dimension",
			a:       make([]float32, 1000),
			b:       make([]float32, 1000),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CosineSimilarity(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && (result < -1.001 || result > 1.001) {
				t.Errorf("cosine similarity out of range: %v", result)
			}
		})
	}
}

func TestNormalize_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		vec     []float32
		wantErr bool
	}{
		{
			name:    "very large vector",
			vec:     []float32{1e10, 1e10},
			wantErr: false,
		},
		{
			name:    "very small vector",
			vec:     []float32{1e-10, 1e-10},
			wantErr: false,
		},
		{
			name:    "negative values",
			vec:     []float32{-3, -4},
			wantErr: false,
		},
		{
			name:    "single element",
			vec:     []float32{5.0},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.vec)
			if (err != nil) != tt.wantErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if !tt.wantErr {
				mag, _ := Magnitude(result)
				if math.Abs(mag-1.0) > 0.0001 {
					t.Errorf("normalized vector magnitude = %v, want 1.0", mag)
				}
			}
		})
	}
}

func TestEuclideanDistance_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		wantErr  bool
	}{
		{
			name:     "negative values",
			a:        []float32{-1, -1},
			b:        []float32{1, 1},
			expected: 2.8284271247461903,
			wantErr:  false,
		},
		{
			name:     "single dimension",
			a:        []float32{0},
			b:        []float32{5},
			expected: 5.0,
			wantErr:  false,
		},
		{
			name:     "very close vectors",
			a:        []float32{1.0, 2.0},
			b:        []float32{1.0001, 2.0001},
			expected: 0.0001414213562373085,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EuclideanDistance(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if !tt.wantErr {
				if math.Abs(result-tt.expected) > 0.0001 {
					t.Errorf("EuclideanDistance() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestDotProduct_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		a         []float32
		b         []float32
		expected  float64
		tolerance float64
	}{
		{
			name:      "all zeros",
			a:         []float32{0, 0, 0},
			b:         []float32{0, 0, 0},
			expected:  0.0,
			tolerance: 0.0001,
		},
		{
			name:      "large numbers",
			a:         []float32{1000000, 2000000},
			b:         []float32{3000000, 4000000},
			expected:  1.1e13,
			tolerance: 1e10,
		},
		{
			name:      "fractional values",
			a:         []float32{0.1, 0.2},
			b:         []float32{0.3, 0.4},
			expected:  0.11,
			tolerance: 0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DotProduct(tt.a, tt.b)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if math.Abs(result-tt.expected) > tt.tolerance {
				t.Errorf("DotProduct() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMagnitude_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		vec      []float32
		expected float64
	}{
		{
			name:     "negative values",
			vec:      []float32{-3, -4},
			expected: 5.0,
		},
		{
			name:     "mixed values",
			vec:      []float32{-1, 2, -3},
			expected: 3.7416573867739413,
		},
		{
			name:     "single negative",
			vec:      []float32{-5},
			expected: 5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Magnitude(tt.vec)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("Magnitude() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAdd_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
	}{
		{
			name: "large values",
			a:    []float32{1000000, 2000000},
			b:    []float32{3000000, 4000000},
		},
		{
			name: "opposite signs",
			a:    []float32{5, -5},
			b:    []float32{-5, 5},
		},
		{
			name: "fractional",
			a:    []float32{0.1, 0.2},
			b:    []float32{0.3, 0.4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Add(tt.a, tt.b)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			for i := range result {
				expected := tt.a[i] + tt.b[i]
				if math.Abs(float64(result[i]-expected)) > 0.0001 {
					t.Errorf("position %d: expected %v, got %v", i, expected, result[i])
				}
			}
		})
	}
}

func TestSubtract_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
	}{
		{
			name: "same vector",
			a:    []float32{1, 2, 3},
			b:    []float32{1, 2, 3},
		},
		{
			name: "negative result",
			a:    []float32{1, 2},
			b:    []float32{3, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Subtract(tt.a, tt.b)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			for i := range result {
				expected := tt.a[i] - tt.b[i]
				if math.Abs(float64(result[i]-expected)) > 0.0001 {
					t.Errorf("position %d: expected %v, got %v", i, expected, result[i])
				}
			}
		})
	}
}

func TestMultiply_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		vec    []float32
		scalar float64
	}{
		{
			name:   "multiply by zero",
			vec:    []float32{1, 2, 3},
			scalar: 0.0,
		},
		{
			name:   "multiply by very large number",
			vec:    []float32{1, 2},
			scalar: 1e10,
		},
		{
			name:   "multiply by very small number",
			vec:    []float32{1, 2},
			scalar: 1e-10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Multiply(tt.vec, tt.scalar)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			for i := range result {
				expected := float32(float64(tt.vec[i]) * tt.scalar)
				if math.Abs(float64(result[i]-expected)) > 0.0001 {
					t.Errorf("position %d: expected %v, got %v", i, expected, result[i])
				}
			}
		})
	}
}

func TestMean_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		vecs    [][]float32
		wantErr bool
	}{
		{
			name: "single vector",
			vecs: [][]float32{{1, 2, 3}},
		},
		{
			name: "many vectors",
			vecs: [][]float32{
				{1, 1},
				{2, 2},
				{3, 3},
				{4, 4},
				{5, 5},
			},
		},
		{
			name:    "dimension mismatch",
			vecs:    [][]float32{{1, 2}, {1, 2, 3}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Mean(tt.vecs)
			if (err != nil) != tt.wantErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if !tt.wantErr {
				if len(result) != len(tt.vecs[0]) {
					t.Errorf("wrong dimension: got %d, want %d", len(result), len(tt.vecs[0]))
				}
			}
		})
	}
}

func TestChunkText_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxTokens int
	}{
		{
			name:      "exact boundary",
			text:      string(make([]byte, 200)),
			maxTokens: 50,
		},
		{
			name:      "no sentence boundaries",
			text:      "no punctuation here just words",
			maxTokens: 5,
		},
		{
			name:      "very small max tokens",
			text:      "test",
			maxTokens: 1,
		},
		{
			name:      "text with newlines",
			text:      "line1\nline2\nline3\nline4",
			maxTokens: 10,
		},
		{
			name:      "multiple sentences",
			text:      "First sentence. Second sentence. Third sentence. Fourth sentence.",
			maxTokens: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text, tt.maxTokens)
			if len(chunks) == 0 {
				t.Error("expected at least one chunk")
			}
		})
	}
}
