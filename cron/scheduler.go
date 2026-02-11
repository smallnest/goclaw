package cron

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
	"go.uber.org/zap"
)

// Scheduler 定时任务调度器
type Scheduler struct {
	cron       *Cron
	jobs       map[string]*Job
	bus        *bus.MessageBus
	provider   providers.Provider
	sessionMgr *session.Manager
	mu         sync.RWMutex
	running    bool
}

// Job 定时任务
type Job struct {
	ID         string
	Name       string
	Schedule   string
	Task       string
	TargetChat string
	Enabled    bool
	LastRun    time.Time
	NextRun    time.Time
	RunCount   int
}

// NewScheduler 创建调度器
func NewScheduler(bus *bus.MessageBus, provider providers.Provider, sessionMgr *session.Manager) *Scheduler {
	return &Scheduler{
		cron:       NewCron(),
		jobs:       make(map[string]*Job),
		bus:        bus,
		provider:   provider,
		sessionMgr: sessionMgr,
		running:    false,
	}
}

// Start 启动调度器
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.running = true

	// 启动 cron
	go s.cron.Run()

	logger.Info("Cron scheduler started")

	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.cron.Stop()
	s.running = false

	logger.Info("Cron scheduler stopped")
}

// AddJob 添加任务
func (s *Scheduler) AddJob(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[job.ID]; ok {
		return fmt.Errorf("job %s already exists", job.ID)
	}

	s.jobs[job.ID] = job

	// 如果任务已启用，添加到 cron
	if job.Enabled {
		if err := s.scheduleJob(job); err != nil {
			return err
		}
	}

	logger.Info("Cron job added",
		zap.String("job_id", job.ID),
		zap.String("schedule", job.Schedule),
	)

	return nil
}

// scheduleJob 调度任务
func (s *Scheduler) scheduleJob(job *Job) error {
	// 解析 cron 表达式
	schedule, err := Parse(job.Schedule)
	if err != nil {
		return fmt.Errorf("invalid cron schedule: %w", err)
	}

	// 添加到 cron
	s.cron.Schedule(schedule, s.createJobFunc(job), job.ID)

	// 计算下次运行时间
	job.NextRun = schedule.Next(time.Now())

	return nil
}

// createJobFunc 创建任务函数
func (s *Scheduler) createJobFunc(job *Job) func() {
	return func() {
		if !job.Enabled {
			return
		}

		logger.Info("Running cron job",
			zap.String("job_id", job.ID),
			zap.String("name", job.Name),
		)

		// 执行任务
		if err := s.executeJob(job); err != nil {
			logger.Error("Cron job execution failed",
				zap.String("job_id", job.ID),
				zap.Error(err),
			)
		}

		// 更新任务状态
		job.LastRun = time.Now()
		job.RunCount++
	}
}

// executeJob 执行任务
func (s *Scheduler) executeJob(job *Job) error {
	ctx := context.Background()

	// 构建消息
	msg := &bus.InboundMessage{
		Channel:  "cron",
		SenderID: job.ID,
		ChatID:   job.TargetChat,
		Content:  job.Task,
		Metadata: map[string]interface{}{
			"job_id":    job.ID,
			"job_name":  job.Name,
			"scheduled": true,
		},
		Timestamp: time.Now(),
	}

	return s.bus.PublishInbound(ctx, msg)
}

// RemoveJob 移除任务
func (s *Scheduler) RemoveJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[id]; !ok {
		return fmt.Errorf("job %s not found", id)
	}

	// 从 cron 移除
	s.cron.Remove(id)

	delete(s.jobs, id)

	logger.Info("Cron job removed", zap.String("job_id", id))

	return nil
}

// GetJob 获取任务
func (s *Scheduler) GetJob(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[id]
	return job, ok
}

// ListJobs 列出所有任务
func (s *Scheduler) ListJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// EnableJob 启用任务
func (s *Scheduler) EnableJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}

	if job.Enabled {
		return nil
	}

	job.Enabled = true

	// 添加到 cron
	if err := s.scheduleJob(job); err != nil {
		job.Enabled = false
		return err
	}

	logger.Info("Cron job enabled", zap.String("job_id", id))

	return nil
}

// DisableJob 禁用任务
func (s *Scheduler) DisableJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}

	if !job.Enabled {
		return nil
	}

	job.Enabled = false

	// 从 cron 移除
	s.cron.Remove(id)

	logger.Info("Cron job disabled", zap.String("job_id", id))

	return nil
}
