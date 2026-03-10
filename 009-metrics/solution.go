package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type metrics struct {
	InFlight  atomic.Int64
	Processed atomic.Int64
	Errors    atomic.Int64
	Panic     atomic.Int64
}

type Metrics struct {
	InFlight  int64
	Processed int64
	Errors    int64
	Panic     int64
}

type Config struct {
	Workers   int
	JobsBuf   int
	ProcessFn func(Job) (int, error)
}

type Job struct {
	Id    int
	Value int
}

type PanicError struct {
	WorkerId  int
	Recovered any
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("worker-%d panic: %v", e.WorkerId, e.Recovered)
}

type Result struct {
	WorkerId int
	Job      Job
	Value    int
	Err      error
}

type Pool struct {
	jobs    chan Job
	results chan Result
	cfg     Config
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	once    sync.Once

	done chan struct{}

	metrics metrics
}

func NewPool(ctx context.Context, cfg Config) *Pool {
	jobs := make(chan Job, cfg.JobsBuf)
	results := make(chan Result, cfg.Workers)

	ctx, cancel := context.WithCancel(ctx)
	pool := &Pool{
		jobs:    jobs,
		results: results,
		ctx:     ctx,
		cfg:     cfg,
		cancel:  cancel,
		done:    make(chan struct{}, 1),
	}
	for i := 0; i < cfg.Workers; i++ {
		pool.wg.Add(1)
		go pool.startWorker(i)
	}
	go func() {
		pool.wg.Wait()
		close(results)
		pool.done <- struct{}{}
	}()

	return pool
}

func (p *Pool) GetMetrics() Metrics {
	return Metrics{
		InFlight:  p.metrics.InFlight.Load(),
		Processed: p.metrics.Processed.Load(),
		Errors:    p.metrics.Errors.Load(),
		Panic:     p.metrics.Panic.Load(),
	}
}

func (p *Pool) QueueDepth() int {
	return len(p.jobs)
}

func (p *Pool) startWorker(id int) {
	defer p.wg.Done()
	for {
		select {
		case job, ok := <-p.jobs:
			if !ok {
				return
			}

			select {
			case <-p.ctx.Done():
				return
			default:
			}

			result := p.executeTask(id, job)
			select {
			case p.results <- result:
			case <-p.ctx.Done():
				return
			}
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *Pool) executeTask(workerId int, job Job) (result Result) {
	p.metrics.InFlight.Add(1)
	defer func() {
		var panicErr *PanicError
		if result.Err != nil {
			if errors.As(result.Err, &panicErr) {
				p.metrics.Panic.Add(1)
			} else {
				p.metrics.Errors.Add(1)
			}
		}
	}()
	defer func() {
		p.metrics.Processed.Add(1)
		p.metrics.InFlight.Add(-1)
		if r := recover(); r != nil {
			result.Err = &PanicError{
				WorkerId:  workerId,
				Recovered: r,
			}
		}
	}()
	result = Result{WorkerId: workerId, Job: job}

	result.Value, result.Err = p.cfg.ProcessFn(job)
	return
}

func (p *Pool) Submit(job Job) error {
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	default:
	}

	select {
	case p.jobs <- job:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

func (p *Pool) TrySubmit(job Job) bool {
	select {
	case <-p.ctx.Done():
		return false
	default:
	}

	select {
	case p.jobs <- job:
		return true
	case <-p.ctx.Done():
		return false
	default:
		return false
	}
}

func (p *Pool) SubmitWithTimeout(job Job, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	default:
	}

	select {
	case p.jobs <- job:
		return nil
	case <-timer.C:
		return fmt.Errorf("submit timeout after %s: queue %d/%d",
			timeout, len(p.jobs), cap(p.jobs))
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

func (p *Pool) Shutdown() {
	p.once.Do(func() {
		close(p.jobs)
	})
	p.wg.Wait()
}

func (p *Pool) Stop() {
	p.cancel()
	p.wg.Wait()
}

func (p *Pool) Done() <-chan struct{} {
	return p.done
}

func (p *Pool) Results() <-chan Result {
	return p.results
}

func main() {
	numTasks := 100
	numWorkers := 4

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := NewPool(ctx, Config{
		Workers: numWorkers,
		JobsBuf: numTasks,
		ProcessFn: func(job Job) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return process(job)
		},
	})

	go func() {
		defer pool.Shutdown()
		for i := 1; i < numTasks+1; i++ {
			job := Job{Id: i, Value: i}
			if err := pool.Submit(job); err != nil {
				fmt.Printf("submit error: %v\n", err)
				return
			}
		}
	}()

	go func() {
		duration := 50 * time.Millisecond
		ticker := time.NewTicker(duration)
		ticks := 0
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				poolMetrics := pool.GetMetrics()
				queueDepth := pool.QueueDepth()
				fmt.Printf("[t=%dms]   processed=%d   errors=%d  panics=%d  inflight=%d  queue=%d\n",
					int64(ticks+1)*duration.Milliseconds(), poolMetrics.Processed, poolMetrics.Errors, poolMetrics.Panic, poolMetrics.InFlight, queueDepth,
				)
				ticks++
			case <-pool.Done():
				return
			}
		}
	}()

	for range pool.Results() {
	}

	m := pool.GetMetrics()
	fmt.Printf("\n=== Final Metrics ===\n")
	fmt.Printf("processed: %d\n", m.Processed)
	fmt.Printf("errors:    %d\n", m.Errors)
	fmt.Printf("panics:    %d\n", m.Panic)
	fmt.Printf("inflight:  %d\n", m.InFlight)
	fmt.Printf("queue:     %d\n", pool.QueueDepth())

	fmt.Println("main done")
}

func process(job Job) (int, error) {
	if job.Id%5 == 0 {
		panic(fmt.Sprintf("unexpected nil pointer in job: %d", job.Id))
	}
	if job.Id%3 == 0 {
		return 0, fmt.Errorf("job %d failed: value is divisible by 3", job.Id)
	}
	return job.Value * job.Value, nil
}
