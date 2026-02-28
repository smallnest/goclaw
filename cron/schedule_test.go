package cron

import (
	"testing"
	"time"
)

func TestParseCronExpressionNextDayWhenHourAlreadyPassed(t *testing.T) {
	from := time.Date(2026, 2, 28, 9, 52, 48, 0, time.UTC)
	next, err := parseCronExpression("0 8 * * *", from)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	expected := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected.Format(time.RFC3339), next.Format(time.RFC3339))
	}
}

func TestCalculateNextRunEveryRequiresPositiveDuration(t *testing.T) {
	job := &Job{
		Schedule: Schedule{
			Type:          ScheduleTypeEvery,
			EveryDuration: 0,
		},
	}

	_, err := job.CalculateNextRun(time.Now())
	if err == nil {
		t.Fatalf("expected error for zero every duration")
	}
}

func TestCalculateNextRunCronRequiresExpression(t *testing.T) {
	job := &Job{
		Schedule: Schedule{
			Type:           ScheduleTypeCron,
			CronExpression: "",
		},
	}

	_, err := job.CalculateNextRun(time.Now())
	if err == nil {
		t.Fatalf("expected error for empty cron expression")
	}
}
