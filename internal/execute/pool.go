// pool.go manages sequential and parallel execution pools for bead processing.
package execute

import "fmt"

// ExecutionPool tracks progress across all beads in an execution run.
type ExecutionPool struct {
	Total     int
	Completed int
	Failed    int
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
	p.Completed++
}

// RecordStuck increments the stuck count.
func (p *ExecutionPool) RecordStuck() {
	p.Stuck++
}

// RecordSkip increments the skipped count.
func (p *ExecutionPool) RecordSkip() {
	p.Skipped++
}

// Progress returns a formatted progress string like "[2/5]".
func (p *ExecutionPool) Progress() string {
	done := p.Completed + p.Stuck + p.Skipped
	return fmt.Sprintf("[%d/%d]", done, p.Total)
}

// IsComplete returns true if all beads have been processed
// (completed, stuck, or skipped).
func (p *ExecutionPool) IsComplete() bool {
	return (p.Completed + p.Stuck + p.Skipped) >= p.Total
}
