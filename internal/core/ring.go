package core

import "time"

// Ring is a fixed-capacity time-series buffer. Not safe for concurrent use;
// the registry serializes access.
type Ring struct {
	pts  []Point
	head int // next write position
	n    int
}

func NewRing(capacity int) *Ring {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring{pts: make([]Point, capacity)}
}

func (r *Ring) Push(ts time.Time, v float64) {
	r.pts[r.head] = Point{Ts: ts.UnixMilli(), Value: v}
	r.head = (r.head + 1) % len(r.pts)
	if r.n < len(r.pts) {
		r.n++
	}
}

// Points returns samples oldest to newest.
func (r *Ring) Points() []Point {
	out := make([]Point, r.n)
	start := r.head - r.n
	if start < 0 {
		start += len(r.pts)
	}
	for i := 0; i < r.n; i++ {
		out[i] = r.pts[(start+i)%len(r.pts)]
	}
	return out
}

func (r *Ring) Len() int { return r.n }
