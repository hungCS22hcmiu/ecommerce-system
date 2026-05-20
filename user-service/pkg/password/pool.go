package password

import (
	"context"
	"errors"
	"runtime"

	"golang.org/x/crypto/bcrypt"
)

// ErrBcryptOverload is returned by Verify when the worker queue is full.
var ErrBcryptOverload = errors.New("bcrypt worker pool overloaded")

type verifyRequest struct {
	hash     string
	password string
	reply    chan error
}

// Pool serializes bcrypt verification calls across a bounded goroutine pool
// sized to runtime.NumCPU(). This prevents bcrypt from saturating all goroutines
// and allows clean load shedding when the queue is full.
type Pool struct {
	jobs chan verifyRequest
	done chan struct{}
}

// NewPool creates a Pool with the given job queue size. Call Start() before use.
func NewPool(queueSize int) *Pool {
	return &Pool{
		jobs: make(chan verifyRequest, queueSize),
		done: make(chan struct{}),
	}
}

// Start launches runtime.NumCPU() worker goroutines.
func (p *Pool) Start() {
	for range runtime.NumCPU() {
		go p.worker()
	}
}

// Stop signals all workers to exit. Call after the HTTP server has shut down
// so no Verify() calls can arrive after Stop returns.
func (p *Pool) Stop() {
	close(p.done)
}

func (p *Pool) worker() {
	for {
		select {
		case <-p.done:
			return
		case req := <-p.jobs:
			req.reply <- bcrypt.CompareHashAndPassword([]byte(req.hash), []byte(req.password))
		}
	}
}

// Verify enqueues a bcrypt verification job and blocks until the result arrives
// or ctx is cancelled. Returns ErrBcryptOverload if the queue is full.
func (p *Pool) Verify(ctx context.Context, hash, password string) error {
	reply := make(chan error, 1)

	select {
	case p.jobs <- verifyRequest{hash: hash, password: password, reply: reply}:
	default:
		return ErrBcryptOverload
	}

	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
