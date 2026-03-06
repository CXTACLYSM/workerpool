package main

import (
	"fmt"
	"sync"
)

func main() {
	numWorkers := 3
	numTasks := 9

	tasksCh := make(chan string, numWorkers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go startWorker(i, tasksCh, &wg)
	}

	go func() {
		defer close(tasksCh)
		for i := 0; i < numTasks; i++ {
			tasksCh <- fmt.Sprintf("task-%d", i)
		}
	}()

	wg.Wait()
	fmt.Println("main done")
}

func startWorker(id int, jobs <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs {
		fmt.Printf("worker-%d processsing job: %s\n", id, job)
	}
	fmt.Printf("worker-%d done\n", id)
}
