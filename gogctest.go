package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/hashicorp/golang-lru"
	"golang.org/x/time/rate"
	"log"
	"math/rand"
	"sync"
	"time"
)

const (
	threadCount = 1000
	// Total size of LRU across all threads.
	defaultTotalLruSize = 10000000
	// Total rate of LRU adds across all threads.
	defaultTotalAddRate = 10000

	pauseReportThreshold        = "1ms"
	hiccupDetectorSleepDuration = "1ms"
	hiccupDetectorThreshold     = "2ms"

	// Delay after starting the tool; allows all lru workers to start. Actual delay
	// will be jittered on a per-thread basis to avoid thundering herd causing huge
	// CPU burst in the beginning.
	startupDelay = "5s"
)

var totalAddRate float64
var addRateBurst int
var totalLruSize int

func main() {
	flag.Float64Var(&totalAddRate, "addrate", defaultTotalAddRate, "Number of LRU adds per second.")
	flag.IntVar(&totalLruSize, "lrusize", defaultTotalLruSize, "LRU size (number of entries)")
	flag.Parse()

	// LRU implementation panics on use if size is 0, so don't allow sizes smaller than
	// thread count since that would lead to 0 per thread.
	if totalLruSize < threadCount {
		log.Fatal("lru size must be >= thread count")
	}

	addRateBurst = int(totalAddRate/threadCount) / 10

	wg := sync.WaitGroup{}

	wg.Add(1)
	go hiccupDetector()

	log.Printf("starting %v workers", threadCount)
	wg.Add(threadCount)
	for i := 0; i < threadCount; i++ {
		go lruWorker()
	}
	wg.Wait()
}

func lruWorker() {
	startupDelay, err := time.ParseDuration(startupDelay)
	if err != nil {
		log.Panic("couldn't parse duration")
	}
	time.Sleep(time.Duration(int64(float64(startupDelay.Nanoseconds()) +
		float64(startupDelay.Nanoseconds())*
			rand.Float64())))

	lruSize := totalLruSize / threadCount
	l, _ := lru.New(lruSize)
	rateLimiter := rate.NewLimiter(rate.Limit(float64(totalAddRate)/float64(threadCount)), addRateBurst)
	pauseReportThreshold, err := time.ParseDuration(pauseReportThreshold)
	if err != nil {
		log.Panic("couldn't parse duration")
	}

	for i := uint64(0); i < (1 << 63); i++ {
		beforeTime := time.Now()
		l.Add(i, fmt.Sprintf("val%d", i))
		elapsedDuration := time.Since(beforeTime)

		if elapsedDuration.Nanoseconds() > pauseReportThreshold.Nanoseconds() {
			log.Printf("lru add latency %v ms", elapsedDuration.Nanoseconds()/1000000)
		}

		err := rateLimiter.Wait(context.TODO())
		if err != nil {
			panic(err)
		}
	}
}

func hiccupDetector() {
	sleepDuration, err := time.ParseDuration(hiccupDetectorSleepDuration)
	if err != nil {
		panic("ParseDuration failed")
	}
	thresholdDuration, err := time.ParseDuration(hiccupDetectorThreshold)
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
