package async

import (
	"context"

	"golang.org/x/sync/errgroup"
)

type (
	Pool struct {
		taskCh chan asyncJobFn
	}

	asyncJobFn func(context.Context) error
)

func NewPool() Pool {
	return Pool{
		taskCh: make(chan asyncJobFn),
	}
}

func (p Pool) Run(ctx context.Context, workersNum int) error {
	errG, errCtx := errgroup.WithContext(ctx)

	for i := 0; i < workersNum; i++ {
		errG.Go(func() error {
			for t := range p.taskCh {
				select {
				case <-errCtx.Done():
					return nil
				default:
					if err := t(errCtx); err != nil {
						return err
					}
				}
			}

			return nil
		})
	}

	return errG.Wait()
}

// Enqueue adds new task to the tasks queue.
func (p Pool) Enqueue(task asyncJobFn) {
	p.taskCh <- task
}
