package main

import (
	"context"
	"sync"
)

type Job struct {
	Id int
}

type Result struct {
}

type Config struct {
	buf     int
	workers int
}

type Pool struct {
	jobs    chan Job
	results chan Result

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once

	done chan struct{}
}

func NewPool(cfg Config) *Pool {
	jobs := make(chan Job, cfg.buf)
	results := make(chan Result, cfg.workers)

	pool := &Pool{
		jobs:    jobs,
		results: results,
		done:    make(chan struct{}),
	}

	for i := 0; i < cfg.workers; i++ {
		go pool.startWorker()
	}

	return pool
}

func (p *Pool) startWorker() {
	defer p.wg.Done()
	select {
	case job, ok := <-p.jobs:
		if !ok {
			return
		}

	case <-p.ctx.Done():
		return
	}
}
