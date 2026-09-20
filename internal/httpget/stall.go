package httpget

import (
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type stallReader struct {
	r    io.Reader
	n    atomic.Int64
	flag atomic.Bool
	done chan struct{}
	once sync.Once
}

func newStallReader(r io.Reader, limit, window int, tick time.Duration, cancel func()) *stallReader {
	s := &stallReader{r: r, done: make(chan struct{})}
	if limit <= 0 || window <= 0 || tick <= 0 {
		return s
	}
	go s.watch(limit, window, tick, cancel)
	return s
}

func (s *stallReader) watch(limit, window int, tick time.Duration, cancel func()) {
	ticks := int(math.Ceil(float64(window) / tick.Seconds()))
	if ticks < 1 {
		ticks = 1
	}
	threshold := int64(limit) * int64(window)

	ring := make([]int64, ticks)
	idx, filled := 0, 0
	last := s.n.Load()

	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			cur := s.n.Load()
			ring[idx] = cur - last
			last = cur
			idx = (idx + 1) % ticks
			if filled < ticks {
				filled++
				continue
			}
			var sum int64
			for _, v := range ring {
				sum += v
			}
			if sum < threshold {
				s.flag.Store(true)
				cancel()
				return
			}
		}
	}
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.n.Add(int64(n))
	}
	return n, err
}

func (s *stallReader) stop() { s.once.Do(func() { close(s.done) }) }

func (s *stallReader) stalled() bool { return s.flag.Load() }
