package debugsession

import (
	"fmt"
	"math"

	"github.com/shirou/gopsutil/process"
)

// killTree kills the process and everything below it, each child before its parent. A process that cannot be looked
// up is an error; failures to list children or to kill are ignored, as a process may already be gone.
func killTree(pid int) error {
	if pid <= 0 || pid > math.MaxInt32 {
		return fmt.Errorf("failed to get process information for pid %d: not a process id", pid)
	}
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return fmt.Errorf("failed to get process information for pid %d: %w", pid, err)
	}
	killWithChildren(p)
	return nil
}

func killWithChildren(p *process.Process) {
	children, _ := p.Children()
	for _, c := range children {
		killWithChildren(c)
	}
	_ = p.Kill()
}
