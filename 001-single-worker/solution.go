package main

import (
	"fmt"
	"sync"
)

func main() {
	numTasks := 5

	tasks := make(chan string)

	var wg sync.WaitGroup
	wg.Add(1)
	go startWorker(0, tasks, &wg)

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
	fmt.Printf("worker-%d processing task: %s\n", workerId, task)
}
