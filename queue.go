package sightpane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"runtime"
	"sync"
	"time"
)

type eventQueue struct {
	transport Transport
	options   Options

	sessionID   string
	sessionTime string
	deviceJSON  json.RawMessage

	itemCh  chan json.RawMessage
	flushCh chan chan error
	stopCh  chan struct{}
	doneCh  chan struct{}

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func newEventQueue(t Transport, opts Options) *eventQueue {
	sessID := generateID(16)
	sessTime := nowISO8601()

	dev := Device{
		Platform:         "go",
		PlatformCategory: "backend",
		AppType:          opts.AppType,
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		Hostname:         opts.ServerName,
		GoVersion:        runtime.Version(),
		Environment:      opts.Environment,
		Release:          opts.Release,
	}
	devJSON, _ := json.Marshal(dev)

	q := &eventQueue{
		transport:   t,
		options:     opts,
		sessionID:   sessID,
		sessionTime: sessTime,
		deviceJSON:  devJSON,
		itemCh:      make(chan json.RawMessage, opts.MaxQueueSize),
		flushCh:     make(chan chan error),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	go q.worker()
	return q
}

func (q *eventQueue) Enqueue(item json.RawMessage) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.mu.Unlock()

	select {
	case q.itemCh <- item:
	default:
		// Queue is full; discard oldest to maintain throughput
		select {
		case <-q.itemCh:
		default:
		}
		select {
		case q.itemCh <- item:
		default:
		}
	}
}

func (q *eventQueue) worker() {
	defer close(q.doneCh)

	ticker := time.NewTicker(q.options.FlushInterval)
	defer ticker.Stop()

	var batch []json.RawMessage

	sendBatch := func(ctx context.Context) error {
		if len(batch) == 0 {
			return nil
		}
		itemsToSend := batch
		batch = nil

		env := &Envelope{
			SDK: SDKInfo{
				Name:    SDKName,
				Version: SDKVersion,
			},
			Session: SessionInfo{
				ID:        q.sessionID,
				StartedAt: q.sessionTime,
				Device:    q.deviceJSON,
			},
			Items: itemsToSend,
		}

		err := q.transport.Send(ctx, env)
		if err != nil && q.options.Debug {
			log.Printf("sightpane: failed to dispatch envelope: %v", err)
		}
		return err
	}

	for {
		select {
		case item := <-q.itemCh:
			batch = append(batch, item)
			if len(batch) >= q.options.MaxBatchSize {
				_ = sendBatch(context.Background())
			}

		case <-ticker.C:
			if len(batch) > 0 {
				_ = sendBatch(context.Background())
			}

		case respCh := <-q.flushCh:
			// Drain all currently available items
		drainLoop:
			for {
				select {
				case item := <-q.itemCh:
					batch = append(batch, item)
				default:
					break drainLoop
				}
			}
			err := sendBatch(context.Background())
			respCh <- err

		case <-q.stopCh:
			// Drain remaining items before exiting
		drainDone:
			for {
				select {
				case item := <-q.itemCh:
					batch = append(batch, item)
				default:
					break drainDone
				}
			}
			if len(batch) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = sendBatch(ctx)
				cancel()
			}
			return
		}
	}
}

// Flush forces an immediate delivery of all queued items.
func (q *eventQueue) Flush(ctx context.Context) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.mu.Unlock()

	resp := make(chan error, 1)
	select {
	case q.flushCh <- resp:
		select {
		case err := <-resp:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops the worker and flushes remaining events.
func (q *eventQueue) Close() error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	q.mu.Unlock()

	close(q.stopCh)
	<-q.doneCh
	return nil
}

func generateID(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
