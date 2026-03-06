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
		defer close(tasks)
		for i := 0; i < numTasks; i++ {
			tasks <- i
		}
	}()

	go func() {
		wg.Wait()
		close(results)
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
		fmt.Printf("worker-%d processing task: %d\n", id, job)
		results <- Result{Value: job * job}
	}
	fmt.Printf("worker-%d done\n", id)
}
