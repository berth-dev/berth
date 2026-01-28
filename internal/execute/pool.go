// pool.go manages sequential and parallel execution pools for bead processing.
package execute

import (
	"fmt"
	"sync"
)

// ExecutionPool tracks progress across all beads in an execution run.
// All methods are thread-safe via mu. In sequential mode the mutex is
// uncontested, adding zero overhead.
type ExecutionPool struct {
	mu        sync.Mutex
	Total     int
	Completed int
	Stuck     int
	Skipped   int
}

// NewExecutionPool creates an ExecutionPool with the given total bead count.
func NewExecutionPool(total int) *ExecutionPool {
	return &ExecutionPool{
		Total: total,
	}
}

// RecordCompletion increments the completed count.
func (p *ExecutionPool) RecordCompletion() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Completed++
}

// RecordStuck increments the stuck count.
func (p *ExecutionPool) RecordStuck() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Stuck++
}

// RecordSkip increments the skipped count.
func (p *ExecutionPool) RecordSkip() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Skipped++
}

// Progress returns a formatted progress string like "[2/5]".
func (p *ExecutionPool) Progress() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	done := p.Completed + p.Stuck + p.Skipped
	return fmt.Sprintf("[%d/%d]", done, p.Total)
}

// IsComplete returns true if all beads have been processed
// (completed, stuck, or skipped).
func (p *ExecutionPool) IsComplete() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return (p.Completed + p.Stuck + p.Skipped) >= p.Total
}
