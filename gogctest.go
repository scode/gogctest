package main

import (
	"context"
	"fmt"
	"github.com/hashicorp/golang-lru"
	"golang.org/x/time/rate"
	"sync"
	"time"
)

const (
	THREAD_COUNT = 1
	// Total size of LRU across all threads.
	TOTAL_LRU_SIZE = 10000000
	// Total rate of LRU adds across all threads.
	TOTAL_ADD_RATE = 10000
	ADD_RATE_BURST = TOTAL_ADD_RATE / 10

	PAUSE_REPORT_THRESHOLD_MILLIS  = 1
	SLOW_PHASE_THRESHOLD           = TOTAL_LRU_SIZE * 2
	HICCUP_DETECTOR_SLEEP_DURATION = "1ms"
	HICCUP_DETECTOR_THRESHOLD      = "2ms"
)

func main() {
	wg := sync.WaitGroup{}

	wg.Add(1)
	go hiccupDetector()

	wg.Add(THREAD_COUNT)
	for i := 0; i < THREAD_COUNT; i++ {
		go lruWorker()
	}
	wg.Wait()
}

func lruWorker() {
	lruSize := TOTAL_LRU_SIZE / THREAD_COUNT
	l, _ := lru.New(lruSize)
	rateLimiter := rate.NewLimiter(TOTAL_ADD_RATE, ADD_RATE_BURST)

	fmt.Println("filling lru")
	for i := uint64(0); i < (1 << 63); i++ {
		startTime := time.Now().UnixNano()
		l.Add(i, fmt.Sprintf("val%s", i))
		stopTime := time.Now().UnixNano()

		elapsedMillis := (stopTime - startTime) / 1000000

		if elapsedMillis > PAUSE_REPORT_THRESHOLD_MILLIS {
			fmt.Printf("lru add took %v ms\n", elapsedMillis)
		}

		if i == uint64(lruSize) {
			fmt.Println("lru filled")
		}

		if i == SLOW_PHASE_THRESHOLD {
			fmt.Println("slow phase starting")
		}

		if i > SLOW_PHASE_THRESHOLD {
			rateLimiter.Wait(context.TODO())
		}
	}
}

func hiccupDetector() {
	sleepDuration, err := time.ParseDuration(HICCUP_DETECTOR_SLEEP_DURATION)
	if err != nil {
		panic("ParseDuration failed")
	}
	thresholdDuration, err := time.ParseDuration(HICCUP_DETECTOR_THRESHOLD)
	if err != nil {
		panic("ParseDuration failed")
	}

	for {
		beforeTime := time.Now()
		time.Sleep(sleepDuration)
		afterTime := time.Now()
		if afterTime.After(beforeTime.Add(thresholdDuration)) {
			fmt.Printf("hiccup: %vms\n",
				(afterTime.Sub(beforeTime).Nanoseconds()-sleepDuration.Nanoseconds())/1000000)
		}
	}
}
