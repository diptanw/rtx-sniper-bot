package async

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Scheduler struct {
	cancels   []context.CancelFunc
	cancelsMu sync.Mutex
	logger    *slog.Logger
}

func NewScheduler(log *slog.Logger) *Scheduler {
	return &Scheduler{
		logger: log,
	}
}

func (p *Scheduler) Schedule(ctx context.Context, interval time.Duration, fn asyncJobFn) {
	p.cancelsMu.Lock()
	defer p.cancelsMu.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	go func(ctx context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := fn(ctx); err != nil {
					p.logger.Error("Job has failed", "err", err)
				}
			}
		}
	}(ctx)

	p.cancels = append(p.cancels, cancel)
}

func (p *Scheduler) Close() {
	p.cancelsMu.Lock()
	defer p.cancelsMu.Unlock()

	for _, cancel := range p.cancels {
		cancel()
	}

	p.cancels = nil
}
