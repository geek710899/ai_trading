package ratelimit

import (
    "sync"
    "time"
)

type Domain int

const (
    IP Domain = iota
    UID
)

type bucket struct {
    capacity int
    window   time.Duration
    mu       sync.Mutex
    entries  []entry
}

type entry struct {
    t   time.Time
    w   int
}

type Config struct {
    IPCapacity  int
    UIDCapacity int
    Window      time.Duration
}

type RateLimiter struct {
    ip  *bucket
    uid *bucket
}

func New(cfg Config) *RateLimiter {
    return &RateLimiter{
        ip:  &bucket{capacity: cfg.IPCapacity, window: cfg.Window},
        uid: &bucket{capacity: cfg.UIDCapacity, window: cfg.Window},
    }
}

func (b *bucket) prune(now time.Time) {
    cutoff := now.Add(-b.window)
    i := 0
    for ; i < len(b.entries); i++ {
        if b.entries[i].t.After(cutoff) {
            break
        }
    }
    if i > 0 {
        b.entries = b.entries[i:]
    }
}

func (b *bucket) used() int {
    total := 0
    for _, e := range b.entries {
        total += e.w
    }
    return total
}

func (b *bucket) acquire(w int) {
    for {
        now := time.Now()
        b.mu.Lock()
        b.prune(now)
        u := b.used()
        if u+w <= b.capacity {
            b.entries = append(b.entries, entry{t: now, w: w})
            b.mu.Unlock()
            return
        }
        sleep := time.Millisecond * 200
        b.mu.Unlock()
        time.Sleep(sleep)
    }
}

func (rl *RateLimiter) Acquire(d Domain, w int) {
    if w <= 0 {
        return
    }
    if d == IP {
        rl.ip.acquire(w)
    } else {
        rl.uid.acquire(w)
    }
}

