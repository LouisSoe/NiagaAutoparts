package worker

import (
	"context"
	"sync"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"go.uber.org/zap"
)

// Pool manages a fixed set of goroutines that process incoming messages from
// any provider (Fonnte, Telegram, …).
type Pool struct {
	jobQueue     chan model.WorkerJob
	processor    *service.MessageProcessor
	messagingSvc model.MessageSender // used only for panic-recovery fallback
	poolSize     int
	logger       *zap.Logger
	wg           sync.WaitGroup
	stopCh       chan struct{}
}

// NewPool creates a worker pool with the given concurrency and queue capacity.
func NewPool(
	poolSize, queueSize int,
	processor *service.MessageProcessor,
	messagingSvc model.MessageSender,
	logger *zap.Logger,
) *Pool {
	return &Pool{
		jobQueue:     make(chan model.WorkerJob, queueSize),
		processor:    processor,
		messagingSvc: messagingSvc,
		poolSize:     poolSize,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}
}

// Start launches all worker goroutines.
func (p *Pool) Start(ctx context.Context) {
	p.logger.Info("starting worker pool", zap.Int("workers", p.poolSize))
	for i := 0; i < p.poolSize; i++ {
		p.wg.Add(1)
		workerID := i
		go p.runWorker(ctx, workerID)
	}
}

// Stop gracefully shuts down all workers.
func (p *Pool) Stop() {
	p.logger.Info("stopping worker pool")
	close(p.stopCh)
	close(p.jobQueue)
	p.wg.Wait()
	p.logger.Info("worker pool stopped")
}

// Dispatch enqueues a job. Returns false if the queue is full (back-pressure).
func (p *Pool) Dispatch(job model.WorkerJob) bool {
	select {
	case p.jobQueue <- job:
		return true
	default:
		p.logger.Warn("worker queue full, dropping job",
			zap.String("sender", job.Payload.Sender),
			zap.String("platform", string(job.Payload.Platform)),
		)
		return false
	}
}

// QueueLen returns the current number of pending jobs.
func (p *Pool) QueueLen() int {
	return len(p.jobQueue)
}

// ─── Internal ─────────────────────────────────────────────────────────────────

func (p *Pool) runWorker(ctx context.Context, id int) {
	defer p.wg.Done()
	p.logger.Debug("worker started", zap.Int("id", id))

	for {
		select {
		case job, ok := <-p.jobQueue:
			if !ok {
				p.logger.Debug("worker shutting down", zap.Int("id", id))
				return
			}
			p.processJob(ctx, id, job)
		case <-p.stopCh:
			return
		}
	}
}

func (p *Pool) processJob(ctx context.Context, workerID int, job model.WorkerJob) {
	start := time.Now()
	sender := job.Payload.Sender

	// Give each job a generous timeout so AI calls don't get cancelled
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	p.logger.Info("processing job",
		zap.Int("worker", workerID),
		zap.String("platform", string(job.Payload.Platform)),
		zap.String("sender", sender),
		zap.String("msg_preview", truncate(job.Payload.Message, 40)),
	)

	// Recover from panics to keep the worker alive
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("worker panic recovered",
				zap.Int("worker", workerID),
				zap.Any("panic", r),
			)
			_ = p.messagingSvc.SendText(jobCtx, job.Payload.Platform, sender,
				"⚠️ Terjadi kesalahan. Silakan coba lagi atau ketik MENU.")
		}
	}()

	p.processor.Process(jobCtx, job.Payload)

	p.logger.Info("job processed",
		zap.Int("worker", workerID),
		zap.Duration("elapsed", time.Since(start)),
	)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}