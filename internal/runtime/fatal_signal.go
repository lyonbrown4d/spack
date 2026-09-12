package runtime

import (
	"sync"

	"github.com/samber/oops"
)

// FatalSignal publishes the first unrecoverable runtime failure to the command runner.
type FatalSignal struct {
	once sync.Once
	mu   sync.RWMutex
	done chan struct{}
	err  error
}

// NewFatalSignal creates a process-lifetime fatal signal.
func NewFatalSignal() *FatalSignal {
	return &FatalSignal{done: make(chan struct{})}
}

// Report publishes err once. Nil errors are ignored.
func (s *FatalSignal) Report(err error) {
	if s == nil || err == nil {
		return
	}
	s.once.Do(func() {
		s.mu.Lock()
		s.err = oops.In("runtime").Owner("fatal signal").Wrap(err)
		s.mu.Unlock()
		close(s.done)
	})
}

// Done closes when a fatal error has been reported.
func (s *FatalSignal) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.done
}

// Err returns the reported fatal error, or nil before Done closes.
func (s *FatalSignal) Err() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}
