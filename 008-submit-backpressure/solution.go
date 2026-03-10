package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

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
	}
	for i := 0; i < cfg.Workers; i++ {
		pool.wg.Add(1)
		go pool.startWorker(i)
	}
	go func() {
		pool.wg.Wait()
		close(results)
	}()

	return pool
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
	defer func() {
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

func (p *Pool) Results() <-chan Result {
	return p.results
}

func runScenarioA(ctx context.Context, numWorkers, numTasks, jobsBuf int) {
	pool := NewPool(ctx, Config{
		Workers: numWorkers,
		JobsBuf: jobsBuf,
		ProcessFn: func(job Job) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return process(job)
		},
	})

	go func() {
		defer pool.Shutdown()
		for i := 1; i < numTasks+1; i++ {
			start := time.Now()
			job := Job{Id: i, Value: i}
			if err := pool.Submit(job); err != nil {
				fmt.Printf("submit error: %v\n", err)
				return
			}
			ms := time.Since(start).Milliseconds()

			comment := "buffer free"
			if ms > 0 {
				comment = "waited for worker"
			}

			fmt.Printf("submit job-%d: %d ms (%s)\n", job.Id, ms, comment)
		}
	}()

	for result := range pool.Results() {
		fmt.Printf("  result job-%d: value=%d err=%v\n",
			result.Job.Id, result.Value, result.Err)
	}
}

func runScenarioB(ctx context.Context, numWorkers, numTasks, jobsBuf int) {
	pool := NewPool(ctx, Config{
		Workers: numWorkers,
		JobsBuf: jobsBuf,
		ProcessFn: func(job Job) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return process(job)
		},
	})

	accepted := 0
	rejected := 0
	for i := 1; i <= numTasks; i++ {
		if pool.TrySubmit(Job{Id: i, Value: i}) {
			accepted++
		} else {
			rejected++
		}
	}

	go pool.Shutdown()
	for result := range pool.Results() {
		fmt.Printf("  result job-%d: value=%d err=%v\n",
			result.Job.Id, result.Value, result.Err)
	}

	fmt.Printf("  accepted: %d / %d\n", accepted, numTasks)
	fmt.Printf("  rejected: %d / %d\n", rejected, numTasks)
}

func runScenarioC(ctx context.Context, numWorkers, numTasks, jobsBuf int) {
	pool := NewPool(ctx, Config{
		Workers: numWorkers,
		JobsBuf: jobsBuf,
		ProcessFn: func(job Job) (int, error) {
			time.Sleep(200 * time.Millisecond)
			return process(job)
		},
	})

	go func() {
		defer pool.Shutdown()
		for i := 1; i < numTasks+1; i++ {
			job := Job{Id: i, Value: i}
			if err := pool.SubmitWithTimeout(job, 150*time.Millisecond); err != nil {
				fmt.Printf("job-%d: error: %s\n", job.Id, err.Error())
			} else {
				fmt.Printf("job-%d: submitted OK\n", job.Id)
			}
		}
	}()

	for result := range pool.Results() {
		fmt.Printf("  result job-%d: value=%d err=%v\n",
			result.Job.Id, result.Value, result.Err)
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("[Scenario A: Backpressure]")
	runScenarioA(ctx, 2, 10, 3)

	fmt.Println("\n[Scenario B: TrySubmit]")
	runScenarioB(ctx, 2, 10, 3)

	fmt.Println("\n[Scenario C: SubmitWithTimeout]")
	runScenarioC(ctx, 1, 5, 2)

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
