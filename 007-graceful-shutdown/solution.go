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

func runScenarioA(ctx context.Context, numWorkers, numTasks int) {
	pool := NewPool(ctx, Config{
		Workers:   numWorkers,
		JobsBuf:   numTasks,
		ProcessFn: process,
	})

	go func() {
		defer pool.Shutdown()
		for i := 1; i < numTasks+1; i++ {
			if err := pool.Submit(Job{Id: i, Value: i}); err != nil {
				fmt.Printf("submit error: %v\n", err)
				return
			}
		}
	}()

	count := 0
	for result := range pool.Results() {
		fmt.Printf("  job %d: value=%d err=%v\n", result.Job.Id, result.Value, result.Err)
		count++
	}

	fmt.Printf("processed %d / %d\n", count, numTasks)
}

func runScenarioB(ctx context.Context, numWorkers, numTasks int) {
	pool := NewPool(ctx, Config{
		Workers:   numWorkers,
		JobsBuf:   numTasks,
		ProcessFn: process,
	})

	go func() {
		for i := 1; i < numTasks+1; i++ {
			if err := pool.Submit(Job{Id: i, Value: i}); err != nil {
				fmt.Printf("submit error: %v\n", err)
				return
			}
		}
	}()

	time.AfterFunc(50*time.Millisecond, func() {
		pool.Stop()
	})

	count := 0
	for result := range pool.Results() {
		fmt.Printf("  job %d: value=%d err=%v\n", result.Job.Id, result.Value, result.Err)
		count++
	}

	fmt.Printf("processed %d / %d\n", count, numTasks)
}

func main() {
	numTasks := 12
	numWorkers := 3

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Println("=== Scenario A: Graceful Shutdown ===")
	runScenarioA(ctx, numWorkers, numTasks)

	fmt.Println("\n=== Scenario B: Stop ===")
	runScenarioB(ctx, numWorkers, numTasks)

	fmt.Println("main done")
}

func process(job Job) (int, error) {
	time.Sleep(30 * time.Millisecond)
	if job.Id%5 == 0 {
		panic(fmt.Sprintf("unexpected nil pointer in job: %d", job.Id))
	}
	if job.Id%3 == 0 {
		return 0, fmt.Errorf("job %d failed: value is divisible by 3", job.Id)
	}
	return job.Value * job.Value, nil
}
