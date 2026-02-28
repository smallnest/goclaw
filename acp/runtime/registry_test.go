package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterAcpRuntimeBackend tests backend registration.
func TestRegisterAcpRuntimeBackend(t *testing.T) {
	// Create a mock backend
	mockBackend := &AcpRuntimeBackend{
		ID: "test-backend",
		Runtime: &mockRuntime{},
		Healthy: func() bool {
			return true
		},
	}

	// Register the backend
	err := RegisterAcpRuntimeBackend(*mockBackend)
	require.NoError(t, err)

	// Get the backend
	retrieved := GetAcpRuntimeBackend("test-backend")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-backend", retrieved.ID)
	assert.True(t, retrieved.Healthy())
	assert.NotNil(t, retrieved.Runtime)

	// Test case-insensitive lookup
	retrieved = GetAcpRuntimeBackend("TEST-BACKEND")
	assert.NotNil(t, retrieved)

	// Test RequireAcpRuntimeBackend
	required, err := RequireAcpRuntimeBackend("test-backend")
	require.NoError(t, err)
	assert.NotNil(t, required)
	assert.Equal(t, "test-backend", required.ID)

	// Clean up
	UnregisterAcpRuntimeBackend("test-backend")
	retrieved = GetAcpRuntimeBackend("test-backend")
	assert.Nil(t, retrieved)
}

// TestRegisterAcpRuntimeBackendErrors tests error cases in backend registration.
func TestRegisterAcpRuntimeBackendErrors(t *testing.T) {
	tests := []struct {
		name    string
		backend AcpRuntimeBackend
		wantErr bool
	}{
		{
			name: "empty ID",
			backend: AcpRuntimeBackend{
				ID:      "",
				Runtime: &mockRuntime{},
			},
			wantErr: true,
		},
		{
			name: "nil runtime",
			backend: AcpRuntimeBackend{
				ID:      "test",
				Runtime: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterAcpRuntimeBackend(tt.backend)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetAcpRuntimeBackend tests getting backends.
func TestGetAcpRuntimeBackend(t *testing.T) {
	// Register a healthy backend
	healthyBackend := &AcpRuntimeBackend{
		ID:      "healthy",
		Runtime: &mockRuntime{},
		Healthy: func() bool { return true },
	}
	require.NoError(t, RegisterAcpRuntimeBackend(*healthyBackend))

	// Register an unhealthy backend
	unhealthyBackend := &AcpRuntimeBackend{
		ID:      "unhealthy",
		Runtime: &mockRuntime{},
		Healthy: func() bool { return false },
	}
	require.NoError(t, RegisterAcpRuntimeBackend(*unhealthyBackend))

	// Test getting specific backend
	retrieved := GetAcpRuntimeBackend("healthy")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "healthy", retrieved.ID)

	// Test getting unhealthy backend
	retrieved = GetAcpRuntimeBackend("unhealthy")
	assert.Nil(t, retrieved) // Should not return unhealthy backends

	// Test getting non-existent backend
	retrieved = GetAcpRuntimeBackend("non-existent")
	assert.Nil(t, retrieved)

	// Test getting with empty ID (should return first healthy)
	retrieved = GetAcpRuntimeBackend("")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "healthy", retrieved.ID)

	// Clean up
	UnregisterAcpRuntimeBackend("healthy")
	UnregisterAcpRuntimeBackend("unhealthy")
}

// TestNormalizeBackendID tests backend ID normalization.
func TestNormalizeBackendID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test-backend", "test-backend"},
		{"Test-Backend", "test-backend"},
		{"  TEST  BACKEND  ", "test  backend"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeBackendID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockRuntime is a mock implementation of AcpRuntime for testing.
type mockRuntime struct{}

func (m *mockRuntime) EnsureSession(ctx context.Context, input AcpRuntimeEnsureInput) (AcpRuntimeHandle, error) {
	return AcpRuntimeHandle{}, nil
}

func (m *mockRuntime) RunTurn(ctx context.Context, input AcpRuntimeTurnInput) (<-chan AcpRuntimeEvent, error) {
	return nil, nil
}

func (m *mockRuntime) GetCapabilities(ctx context.Context, handle *AcpRuntimeHandle) (AcpRuntimeCapabilities, error) {
	return AcpRuntimeCapabilities{}, nil
}

func (m *mockRuntime) GetStatus(ctx context.Context, handle AcpRuntimeHandle) (*AcpRuntimeStatus, error) {
	return nil, nil
}

func (m *mockRuntime) SetMode(ctx context.Context, handle AcpRuntimeHandle, mode string) error {
	return nil
}

func (m *mockRuntime) SetConfigOption(ctx context.Context, handle AcpRuntimeHandle, key, value string) error {
	return nil
}

func (m *mockRuntime) Doctor(ctx context.Context) (AcpRuntimeDoctorReport, error) {
	return AcpRuntimeDoctorReport{}, nil
}

func (m *mockRuntime) Cancel(ctx context.Context, handle AcpRuntimeHandle, reason string) error {
	return nil
}

func (m *mockRuntime) Close(ctx context.Context, handle AcpRuntimeHandle, reason string) error {
	return nil
}
