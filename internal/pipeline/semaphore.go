package pipeline

import (
	"sync"

	"github.com/kanthorlabs/sag/internal/core"
)

const messageRateLimited = "too many concurrent requests for this agent"

type semaphore struct {
	limit  int
	mu     sync.Mutex
	active map[string]int
}

func newSemaphore(limit int) *semaphore {
	return &semaphore{limit: limit, active: map[string]int{}}
}

func (s *semaphore) acquire(sub string) (func(), error) {
	if s.limit <= 0 {
		return func() {}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[sub] >= s.limit {
		return nil, core.NewError(core.CodeRateLimited, messageRateLimited)
	}
	s.active[sub]++
	var once sync.Once
	return func() { once.Do(func() { s.release(sub) }) }, nil
}

func (s *semaphore) release(sub string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[sub]--
	if s.active[sub] == 0 {
		delete(s.active, sub)
	}
}
