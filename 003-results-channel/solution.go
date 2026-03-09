package main

import (
	"fmt"
	"sync"
)

type Result struct {
	Value int
}

func main() {
	numWorkers := 3
	numTasks := 9

	tasks := make(chan int, numTasks)
	results := make(chan Result, numWorkers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go startWorker(i, tasks, results, &wg)
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

	sum := 0
	for result := range results {
		fmt.Printf("result: %d\n", result.Value)
		sum += result.Value
	}
	fmt.Printf("sum: %d\n", sum)

	fmt.Println("main done")
}

func startWorker(id int, jobs <-chan int, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs {
		value := executeTask(id, job)
		results <- Result{Value: value}
	}
	fmt.Printf("worker-%d done\n", id)
}

func executeTask(workerId int, task int) int {
	fmt.Printf("worker-%d processing task: %d\n", workerId, task)
	return task * task
}
