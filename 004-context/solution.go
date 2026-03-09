package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Result struct {
	WorkerId int
	Value    int
}

func main() {
	numTasks := 20
	numWorkers := 3

	tasks := make(chan int, numTasks)
	results := make(chan Result, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
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
			tasks <- i
		}
	}()

	for result := range results {
		fmt.Printf("received result from worker-%d: %d\n", result.WorkerId, result.Value)
	}

	fmt.Println("main done")
}

func startWorker(ctx context.Context, id int, jobs <-chan int, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case job, ok := <-jobs:
			if !ok {
				fmt.Printf("worker-%d finished because of jobs chanel is closed", id)
				return
			}

			select {
			case <-ctx.Done():
				fmt.Printf("worker-%d stopped before executing job: %s\n", id, ctx.Err())
				return
			default:
			}

			result := executeTask(id, job)
			select {
			case results <- result:
			case <-ctx.Done():
				fmt.Printf("worker-%d stopped while sending: %s\n", id, ctx.Err())
				return
			}
		case <-ctx.Done():
			fmt.Printf("worker-%d stopped: %s\n", id, ctx.Err())
			return
		}
	}
}

func executeTask(workerId int, task int) Result {
	time.Sleep(50 * time.Millisecond)
	value := task * task
	fmt.Printf("worker-%d processed: %d -> %d\n", workerId, task, value)
	return Result{
		WorkerId: workerId,
		Value:    value,
	}
}
