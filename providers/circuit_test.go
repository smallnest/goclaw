package providers

import (
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute)

	if cb.failureThreshold != 3 {
		t.Errorf("Expected threshold 3, got %d", cb.failureThreshold)
	}
	if cb.timeout != 5*time.Minute {
		t.Errorf("Expected timeout 5m, got %v", cb.timeout)
	}
	if cb.state != CircuitStateClosed {
		t.Errorf("Expected state Closed, got %v", cb.state)
	}
}

func TestCircuitBreakerInitialState(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute)

	if cb.IsOpen() {
		t.Error("Expected circuit to be closed initially")
	}

	if !cb.AllowRequest() {
		t.Error("Expected requests to be allowed initially")
	}

	if cb.GetState() != CircuitStateClosed {
		t.Errorf("Expected state Closed, got %v", cb.GetState())
	}
}

func TestCircuitBreakerOpenOnFailureThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute)

	// Record 3 failures
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Circuit should be open
	if !cb.IsOpen() {
		t.Error("Expected circuit to be open after threshold failures")
	}

	if cb.GetState() != CircuitStateOpen {
		t.Errorf("Expected state Open, got %v", cb.GetState())
	}

	// Requests should be denied
	if cb.AllowRequest() {
		t.Error("Expected requests to be denied when circuit is open")
	}
}

func TestCircuitBreakerResetsOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute)

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.failures != 2 {
		t.Errorf("Expected 2 failures, got %d", cb.failures)
	}

	// Record success - should reset failure count
	cb.RecordSuccess()

	if cb.failures != 0 {
		t.Errorf("Expected failures to reset to 0, got %d", cb.failures)
	}
}

func TestCircuitBreakerHalfOpenRecovery(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if !cb.IsOpen() {
		t.Error("Expected circuit to be open")
	}

	// Wait for timeout to allow transition to half-open
	time.Sleep(150 * time.Millisecond)

	// First RecordSuccess in Open state: transitions to HalfOpen (successCount = 0)
	cb.RecordSuccess()
	if cb.GetState() != CircuitStateHalfOpen {
		t.Errorf("Expected state HalfOpen after first RecordSuccess, got %v", cb.GetState())
	}

	// Second success: HalfOpen, successCount = 1
	cb.RecordSuccess()
	if cb.GetState() != CircuitStateHalfOpen {
		t.Errorf("Expected state HalfOpen after second success, got %v", cb.GetState())
	}

	// Third success: HalfOpen, successCount = 2
	cb.RecordSuccess()
	if cb.GetState() != CircuitStateHalfOpen {
		t.Errorf("Expected state HalfOpen after third success, got %v", cb.GetState())
	}

	// Fourth success: HalfOpen, successCount = 3 -> should close the circuit
	cb.RecordSuccess()
	if cb.GetState() != CircuitStateClosed {
		t.Errorf("Expected state Closed after 4th success (3rd in HalfOpen), got %v", cb.GetState())
	}

	if cb.IsOpen() {
		t.Error("Expected circuit to be closed after recovery")
	}
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Wait for timeout to enter half-open
	time.Sleep(150 * time.Millisecond)

	// Failure in half-open should reopen
	cb.RecordFailure()

	if cb.GetState() != CircuitStateOpen {
		t.Errorf("Expected state Open after failure in half-open, got %v", cb.GetState())
	}

	if !cb.IsOpen() {
		t.Error("Expected circuit to be open after half-open failure")
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute)

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if !cb.IsOpen() {
		t.Error("Expected circuit to be open")
	}

	// Reset
	cb.Reset()

	if cb.IsOpen() {
		t.Error("Expected circuit to be closed after reset")
	}

	if cb.GetState() != CircuitStateClosed {
		t.Errorf("Expected state Closed after reset, got %v", cb.GetState())
	}

	if cb.failures != 0 {
		t.Errorf("Expected failures to be 0 after reset, got %d", cb.failures)
	}
}

func TestCircuitBreakerGetStateInfo(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute)

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()

	info := cb.GetStateInfo()

	if info["state"] != "closed" {
		t.Errorf("Expected state 'closed', got %v", info["state"])
	}

	if info["failures"] != 2 {
		t.Errorf("Expected failures 2, got %v", info["failures"])
	}

	if info["is_open"] != false {
		t.Errorf("Expected is_open false, got %v", info["is_open"])
	}
}
