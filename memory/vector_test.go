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
			name:    "empty vector",
			vec:     []float32{},
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
		name         string
		text         string
		maxTokens    int
		minChunks    int
		maxChunks    int
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
