package limits

type Limiter struct {
	ch chan struct{}
}

func NewLimiter(limit int) *Limiter {
	if limit <= 0 {
		limit = 1
	}
	return &Limiter{ch: make(chan struct{}, limit)}
}

func (l *Limiter) TryAcquire() bool {
	select {
	case l.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *Limiter) Release() {
	select {
	case <-l.ch:
	default:
	}
}

func (l *Limiter) Available() int {
	return cap(l.ch) - len(l.ch)
}

func (l *Limiter) InUse() int {
	return len(l.ch)
}
