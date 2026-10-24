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

type speedWindow struct {
	ring   []int64
	idx    int
	filled int
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
	w := newSpeedWindow(max(int(math.Ceil(float64(window)/tick.Seconds())), 1))
	threshold := int64(limit) * int64(window)
	last := s.n.Load()

	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			cur := s.n.Load()
			full := w.add(cur - last)
			last = cur

			if full && w.sum() < threshold {
				s.flag.Store(true)
				cancel()
				return
			}
		}
	}
}

func newSpeedWindow(ticks int) *speedWindow {
	return &speedWindow{ring: make([]int64, ticks)}
}

func (w *speedWindow) add(delta int64) bool {
	w.ring[w.idx] = delta
	w.idx = (w.idx + 1) % len(w.ring)

	if w.filled < len(w.ring) {
		w.filled++
		return false
	}
	return true
}

func (w *speedWindow) sum() int64 {
	var total int64
	for _, v := range w.ring {
		total += v
	}
	return total
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
