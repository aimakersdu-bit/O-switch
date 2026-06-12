package audit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type Recorder struct {
	cfg    Config
	file   *os.File
	events chan Event
	done   chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed atomic.Bool

	written atomic.Uint64
	dropped atomic.Uint64
	syncs   atomic.Uint64
}

type Stats struct {
	Written    uint64
	Dropped    uint64
	SyncWrites uint64
	QueueDepth int
}

func NewRecorder(cfg Config) (*Recorder, error) {
	cfg = cfg.WithDefaults()
	if !cfg.Enabled {
		return &Recorder{cfg: cfg, done: make(chan struct{})}, nil
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	rec := &Recorder{
		cfg:    cfg,
		file:   file,
		events: make(chan Event, cfg.QueueSize),
		done:   make(chan struct{}),
	}
	rec.wg.Add(1)
	go rec.run()
	return rec, nil
}

func (r *Recorder) Record(event Event) {
	if r == nil || !r.cfg.Enabled || r.closed.Load() {
		return
	}
	event = normalizeEvent(event)
	if cap(r.events) == 0 {
		if r.cfg.OverflowPolicy == OverflowSync {
			r.syncs.Add(1)
			r.write(event)
			return
		}
		r.dropped.Add(1)
		return
	}
	select {
	case r.events <- event:
	default:
		if r.cfg.OverflowPolicy == OverflowSync {
			r.syncs.Add(1)
			r.write(event)
			return
		}
		r.dropped.Add(1)
	}
}

func (r *Recorder) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if !r.closed.CompareAndSwap(false, true) {
		return nil
	}
	if r.cfg.Enabled && r.events != nil {
		close(r.events)
	}
	wait := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(wait)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wait:
	}
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

func (r *Recorder) Stats() Stats {
	if r == nil {
		return Stats{}
	}
	depth := 0
	if r.events != nil {
		depth = len(r.events)
	}
	return Stats{
		Written:    r.written.Load(),
		Dropped:    r.dropped.Load(),
		SyncWrites: r.syncs.Load(),
		QueueDepth: depth,
	}
}

func (r *Recorder) run() {
	defer r.wg.Done()
	if r.events == nil {
		return
	}
	for event := range r.events {
		r.write(event)
	}
}

func (r *Recorder) write(event Event) {
	if r == nil || r.file == nil {
		return
	}
	raw, err := json.Marshal(event)
	if err != nil {
		r.dropped.Add(1)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.file.Write(append(raw, '\n')); err != nil {
		r.dropped.Add(1)
		return
	}
	r.written.Add(1)
}

func normalizeEvent(event Event) Event {
	if event.SchemaVersion == 0 {
		event.SchemaVersion = 1
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	_, date, week := NewEventTime(event.Timestamp)
	if event.Date == "" {
		event.Date = date
	}
	if event.Week == "" {
		event.Week = week
	}
	if event.UsageSource == "" {
		if event.InputTokens == 0 && event.OutputTokens == 0 && event.TotalTokens == 0 {
			event.UsageSource = UsageSourceMissing
		} else {
			event.UsageSource = UsageSourceUpstream
		}
	}
	if event.TotalTokens == 0 {
		event.TotalTokens = event.InputTokens + event.OutputTokens
	}
	return event
}

var ErrRecorderClosed = errors.New("audit recorder closed")
