package main

import (
	"fmt"
	"sync"
)

func main() {
	numTasks := 5

	tasksCh := make(chan string)
	var wg sync.WaitGroup

	wg.Add(1)
	go startWorker(0, tasksCh, &wg)

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
		fmt.Printf("worker-%d processing task: %s\n", id, job)
	}
	fmt.Printf("worker-%d done\n", id)
}
