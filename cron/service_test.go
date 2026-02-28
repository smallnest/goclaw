package cron

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smallnest/goclaw/bus"
)

func TestExecuteJobRejectsConcurrentRun(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cron-service-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := DefaultCronConfig()
	cfg.StorePath = filepath.Join(tempDir, "jobs.json")
	cfg.DefaultTimeout = 100 * time.Millisecond

	// Unbuffered bus without a consumer keeps execution in-flight long enough
	// for concurrent execution checks.
	messageBus := bus.NewMessageBus(0)
	svc, err := NewService(cfg, messageBus)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	job := &Job{
		Name: "concurrency-test",
		Schedule: Schedule{
			Type:          ScheduleTypeEvery,
			EveryDuration: time.Hour,
		},
		State: JobState{Enabled: true},
		Payload: Payload{
			Type:    PayloadTypeAgentTurn,
			Message: "echo test",
		},
	}
	if err := svc.AddJob(job); err != nil {
		t.Fatalf("add job: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- svc.executeJob(context.Background(), job)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		svc.jobsMutex.RLock()
		running := false
		if current, ok := svc.jobs[job.ID]; ok {
			running = current.IsRunning()
		}
		svc.jobsMutex.RUnlock()
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job never entered running state")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := svc.executeJob(context.Background(), job); err == nil {
		t.Fatalf("expected concurrent execution rejection")
	}

	<-firstDone
}
