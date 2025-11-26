package strategy

import (
    "math"
    "time"
)

type series struct {
    buf []float64
    i   int
    n   int
}

func newSeries(cap int) *series { return &series{buf: make([]float64, cap)} }

func (s *series) push(v float64) {
    s.buf[s.i%len(s.buf)] = v
    s.i++
    if s.n < len(s.buf) { s.n++ }
}

func (s *series) meanStd() (float64, float64) {
    if s.n == 0 { return 0, 0 }
    sum := 0.0
    for i := 0; i < s.n; i++ { sum += s.buf[i] }
    m := sum / float64(s.n)
    vs := 0.0
    for i := 0; i < s.n; i++ { d := s.buf[i]-m; vs += d*d }
    std := math.Sqrt(vs / float64(s.n))
    return m, std
}

type symbolState struct {
    basis *series
    lastTrigger time.Time
    cooldown    time.Duration
}

func newSymbolState() *symbolState { return &symbolState{basis: newSeries(120), cooldown: 5 * time.Minute} }

