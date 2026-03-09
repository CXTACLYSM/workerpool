package main

import (
	"fmt"
	"sync"
)

func main() {
	numWorkers := 3
	numTasks := 9

	tasks := make(chan string, numWorkers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go startWorker(i, tasks, &wg)
	}

	go func() {
		defer close(tasks)
		for i := 0; i < numTasks; i++ {
			tasks <- fmt.Sprintf("task-%d", i)
		}
	}()

	wg.Wait()
	fmt.Println("main done")
}

func startWorker(id int, jobs <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs {
		executeTask(id, job)
	}
	fmt.Printf("worker-%d done\n", id)
}

func executeTask(workerId int, task string) {
	fmt.Printf("worker-%d processsing job: %s\n", workerId, task)
}
