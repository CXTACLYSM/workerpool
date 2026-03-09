package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Job struct {
	Id    int
	Value int
}

type PanicError struct {
	WorkerId  int
	Recovered any
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("worker %d panic: %v", e.WorkerId, e.Recovered)
}

type Result struct {
	WorkerId int
	Job      Job
	Value    int
	Err      error
}

func main() {
	numTasks := 10
	numWorkers := 3

	tasks := make(chan Job, numTasks)
	results := make(chan Result, numWorkers)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go startWorker(ctx, i, tasks, results, &wg)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		defer close(tasks)
		for i := 1; i < numTasks+1; i++ {
			tasks <- Job{
				Id:    i,
				Value: i,
			}
		}
	}()

	successfulTasks := make([]Result, 0, numTasks)
	errorTasks := make([]Result, 0, numTasks)
	panicTasks := make([]Result, 0, numTasks)
	for result := range results {
		if result.Err == nil {
			successfulTasks = append(successfulTasks, result)
			continue
		}

		var panicError *PanicError
		if errors.As(result.Err, &panicError) {
			panicTasks = append(panicTasks, result)
			continue
		}
		errorTasks = append(errorTasks, result)
	}

	fmt.Println("=== Successful results ===")
	for _, successfulTask := range successfulTasks {
		fmt.Printf("job %d: %d -> %d\n", successfulTask.Job.Id, successfulTask.Job.Value, successfulTask.Value)
	}

	fmt.Println("=== Errors ===")
	for _, errorTask := range errorTasks {
		fmt.Println(errorTask.Err)
	}

	fmt.Println("=== Panics ===")
	for _, panicTask := range panicTasks {
		fmt.Println(panicTask.Err)
	}

	fmt.Println("=== Stats ===")
	fmt.Printf("successful: %d\n", len(successfulTasks))
	fmt.Printf("errors:     %d\n", len(errorTasks))
	fmt.Printf("panics:     %d\n", len(panicTasks))
	fmt.Printf("total:      %d\n", numTasks)
}

func startWorker(ctx context.Context, id int, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case job, ok := <-jobs:
			if !ok {
				return
			}

			select {
			case <-ctx.Done():
				return
			default:
			}

			result := executeTask(id, job)
			select {
			case results <- result:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func executeTask(workerId int, job Job) (result Result) {
	defer func() {
		if r := recover(); r != nil {
			result.Err = &PanicError{
				WorkerId:  workerId,
				Recovered: r,
			}
		}
	}()
	value, err := process(job)
	result = Result{
		WorkerId: workerId,
		Job:      job,
		Value:    value,
		Err:      err,
	}
	return result
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
