package light

import (
	"context"
	"sync"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

type Runner struct {
	mu           sync.RWMutex
	machine      StateMachine
	driver       Driver
	pixels       int
	snapshot     model.StatusSnapshot
	notification *model.NotificationSnapshot
	currentKey   string
	startedAt    time.Time
}

func NewRunner(machine StateMachine, driver Driver, pixels int) *Runner {
	return &Runner{
		machine: machine,
		driver:  driver,
		pixels:  pixels,
	}
}

func (r *Runner) Update(snapshot model.StatusSnapshot, notification *model.NotificationSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshot = snapshot
	r.notification = notification
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	defer r.driver.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			input := r.currentInput(now)
			output := r.machine.Next(input)

			r.mu.Lock()
			if output.Plan.Key != r.currentKey {
				r.currentKey = output.Plan.Key
				r.startedAt = now
			}
			startedAt := r.startedAt
			r.mu.Unlock()

			output.Plan.Phase = now.Sub(startedAt)
			if err := r.driver.Apply(ctx, output.Plan); err != nil {
				return err
			}
		}
	}
}

func (r *Runner) currentInput(now time.Time) Input {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return Input{
		Snapshot:            r.snapshot,
		PrimaryNotification: r.notification,
		Now:                 now,
		Pixels:              r.pixels,
	}
}
