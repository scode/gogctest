package main

import (
	"context"
	"fmt"
	"github.com/hashicorp/golang-lru"
	"golang.org/x/time/rate"
	"log"
	"math/rand"
	"sync"
	"time"
)

const (
	THREAD_COUNT = 1000
	// Total size of LRU across all threads.
	TOTAL_LRU_SIZE = 10000000
	// Total rate of LRU adds across all threads.
	TOTAL_ADD_RATE = 10000
	ADD_RATE_BURST = TOTAL_ADD_RATE / 10

	PAUSE_REPORT_THRESHOLD         = "1ms"
	HICCUP_DETECTOR_SLEEP_DURATION = "1ms"
	HICCUP_DETECTOR_THRESHOLD      = "2ms"

	// Delay after starting the tool; allows all lru workers to start. Actual delay
	// will be jittered on a per-thread basis to avoid thundering herd causing huge
	// CPU burst in the beginning.
	STARTUP_DELAY = "5s"
)

func main() {
	wg := sync.WaitGroup{}

	wg.Add(1)
	go hiccupDetector()

	log.Printf("starting %v workers", THREAD_COUNT)
	wg.Add(THREAD_COUNT)
	for i := 0; i < THREAD_COUNT; i++ {
		go lruWorker()
	}
	wg.Wait()
}

func lruWorker() {
	startupDelay, err := time.ParseDuration(STARTUP_DELAY)
	if err != nil {
		log.Panic("couldn't parse duration")
	}
	time.Sleep(time.Duration(int64(float64(startupDelay.Nanoseconds()) +
		float64(startupDelay.Nanoseconds())*
			rand.Float64())))

	lruSize := TOTAL_LRU_SIZE / THREAD_COUNT
	l, _ := lru.New(lruSize)
	rateLimiter := rate.NewLimiter(TOTAL_ADD_RATE/THREAD_COUNT, ADD_RATE_BURST)
	pauseReportThreshold, err := time.ParseDuration(PAUSE_REPORT_THRESHOLD)
	if err != nil {
		log.Panic("couldn't parse duration")
	}

	for i := uint64(0); i < (1 << 63); i++ {
		beforeTime := time.Now()
		l.Add(i, fmt.Sprintf("val%s", i))
		elapsedDuration := time.Since(beforeTime)

		if elapsedDuration.Nanoseconds() > pauseReportThreshold.Nanoseconds() {
			log.Printf("lru add latency %v ms", elapsedDuration.Nanoseconds()/1000000)
		}

		if i == uint64(lruSize) {
			log.Print("lru full")
		}

		rateLimiter.Wait(context.TODO())
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
			log.Printf("hiccup: %vms\n",
				(afterTime.Sub(beforeTime).Nanoseconds()-sleepDuration.Nanoseconds())/1000000)
		}
	}
}
