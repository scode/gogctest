package main

import (
	"fmt"
	"github.com/hashicorp/golang-lru"
	"sync"
	"time"
)

const (
	THREAD_COUNT                  = 1
	LRU_SIZE                      = 10000000
	PAUSE_REPORT_THRESHOLD_MILLIS = 1
	SLOW_PHASE_THRESHOLD          = LRU_SIZE * 2
	PAUSE_DURATION                = "5ms"
	PAUSE_INTERVAL                = 1000
)

func main() {
	wg := sync.WaitGroup{}
	wg.Add(THREAD_COUNT)
	for i := 0; i < THREAD_COUNT; i++ {
		go lruWorker()
	}
	wg.Wait()
}

func lruWorker() {
	l, _ := lru.New(LRU_SIZE)
	d, _ := time.ParseDuration(PAUSE_DURATION)

	fmt.Println("filling lru")
	for i := uint64(0); i < (1 << 63); i++ {
		startTime := time.Now().UnixNano()
		l.Add(i, fmt.Sprintf("val%s", i))
		stopTime := time.Now().UnixNano()

		elapsedMillis := (stopTime - startTime) / 1000000

		if elapsedMillis > PAUSE_REPORT_THRESHOLD_MILLIS {
			fmt.Printf("lru add took %v ms\n", elapsedMillis)
		}

		if i == LRU_SIZE {
			fmt.Println("lru filled")
		}

		if i == SLOW_PHASE_THRESHOLD {
			fmt.Println("slow phase starting")
		}

		if i > SLOW_PHASE_THRESHOLD && i%PAUSE_INTERVAL == 0 {
			time.Sleep(d)
		}
	}
}
