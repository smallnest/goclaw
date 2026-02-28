package cli

import "testing"

func TestFormatScheduleFromMapHandlesNilCron(t *testing.T) {
	job := map[string]interface{}{
		"schedule": map[string]interface{}{
			"type": "cron",
			"cron": nil,
		},
	}

	got := formatScheduleFromMap(job)
	if got != "<invalid cron>" {
		t.Fatalf("expected <invalid cron>, got %q", got)
	}
}

func TestFormatScheduleFromMapUsesCronExpressionField(t *testing.T) {
	job := map[string]interface{}{
		"schedule": map[string]interface{}{
			"type":            "cron",
			"cron_expression": "0 8 * * *",
		},
	}

	got := formatScheduleFromMap(job)
	if got != "0 8 * * *" {
		t.Fatalf("expected cron expression, got %q", got)
	}
}

func TestFormatScheduleFromMapUsesEveryDurationMsField(t *testing.T) {
	job := map[string]interface{}{
		"schedule": map[string]interface{}{
			"type":              "every",
			"every_duration_ms": float64(300000),
		},
	}

	got := formatScheduleFromMap(job)
	if got != "every 5m0s" {
		t.Fatalf("expected every 5m0s, got %q", got)
	}
}

func TestPrintJobNoPanicWhenStateMissing(t *testing.T) {
	job := map[string]interface{}{
		"id":         "job-1",
		"name":       "test",
		"schedule":   map[string]interface{}{"type": "every", "every": "1h"},
		"created_at": "2026-01-01T00:00:00Z",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("printJob panicked: %v", r)
		}
	}()

	printJob(job)
}
