//go:build windows

package bot

import (
	"time"

	"golang.org/x/sys/windows"
)

// CPUTime returns total kernel + user CPU time consumed by this process.
// FILETIME units are 100ns ticks.
func CPUTime() time.Duration {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &creation, &exit, &kernel, &user); err != nil {
		return 0
	}
	return filetimeDuration(kernel) + filetimeDuration(user)
}

func filetimeDuration(ft windows.Filetime) time.Duration {
	ticks := (uint64(ft.HighDateTime) << 32) | uint64(ft.LowDateTime)
	return time.Duration(ticks * 100)
}

type cpuSample struct {
	wall time.Time
	cpu  time.Duration
}

type cpuSampler struct {
	last cpuSample
}

func newCPUSampler() *cpuSampler {
	return &cpuSampler{last: cpuSample{wall: time.Now(), cpu: CPUTime()}}
}

func (s *cpuSampler) Usage() float64 {
	now := time.Now()
	cpu := CPUTime()
	wall := now.Sub(s.last.wall)
	if wall <= 0 {
		return 0
	}
	frac := float64(cpu-s.last.cpu) / float64(wall)
	s.last = cpuSample{wall: now, cpu: cpu}
	if frac < 0 {
		return 0
	}
	return frac
}
