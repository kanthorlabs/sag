package pipeline

type semaphore struct {
	limit int
}

func newSemaphore(limit int) *semaphore {
	return &semaphore{limit: limit}
}

func (s *semaphore) acquire(string) (func(), error) {
	return func() {}, nil
}
