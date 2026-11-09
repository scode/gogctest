// Command gogctest runs a workload that is hard on garbage collectors: many
// goroutines, each continuously adding small entries to its own fixed-size
// LRU cache at a rate-limited pace.
//
// The program never exits on its own. It logs two kinds of latency outliers so
// that GC pauses show up without a profiler: slow LRU adds (see
// pauseReportThreshold) and oversleeps of a dedicated timer goroutine (see
// hiccupDetector). Run it with GODEBUG=gctrace=1 to correlate those with GC
// cycles.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

const (
	threadCount = 1000
	// Total size of LRU across all threads.
	defaultTotalLruSize = 10000000
	// Total rate of LRU adds across all threads.
	defaultTotalAddRate = 10000

	// A single LRU add slower than this is logged.
	pauseReportThreshold = time.Millisecond

	// The hiccup detector sleeps for hiccupDetectorSleepDuration in a loop and
	// logs whenever a sleep takes longer than hiccupDetectorThreshold.
	hiccupDetectorSleepDuration = time.Millisecond
	hiccupDetectorThreshold     = 2 * time.Millisecond

	// Delay after starting the tool; allows all lru workers to start. Actual delay
	// will be jittered on a per-thread basis to avoid thundering herd causing huge
	// CPU burst in the beginning.
	startupDelay = 5 * time.Second
)

// Set in main, from command-line flags, before any worker starts, and
// read-only afterwards.
var (
	totalAddRate float64
	addRateBurst int
	totalLruSize int
)

func main() {
	flag.Float64Var(&totalAddRate, "addrate", defaultTotalAddRate, "Number of LRU adds per second.")
	flag.IntVar(&totalLruSize, "lrusize", defaultTotalLruSize, "LRU size (number of entries)")
	flag.Parse()

	// Each worker gets totalLruSize/threadCount entries, and the LRU refuses a
	// size of 0, so the total has to cover at least one entry per worker.
	if totalLruSize < threadCount {
		log.Fatal("lru size must be >= thread count")
	}

	// A limiter with a rate of zero or less never refills, so every worker
	// would block forever after its first add.
	if totalAddRate <= 0 {
		log.Fatal("add rate must be > 0")
	}

	// rate.Limiter.Wait fails outright when asked for more tokens than the
	// burst, so a burst of 0 (any rate under 10 adds/s per worker) would make
	// every worker panic on its first add.
	addRateBurst = max(1, int(totalAddRate/threadCount)/10)

	// Nothing below ever returns, so Wait blocks forever. It is there to keep
	// main alive while the goroutines run.
	var wg sync.WaitGroup
	wg.Go(hiccupDetector)

	log.Printf("starting %v workers", threadCount)
	for range threadCount {
		wg.Go(lruWorker)
	}
	wg.Wait()
}

// lruWorker owns one LRU cache and adds a fresh entry to it, forever, at
// 1/threadCount of the total add rate. Keys never repeat, so once the cache is
// full every add evicts the oldest entry; entries therefore die old, which is
// the worst case for a generational collector.
func lruWorker() {
	time.Sleep(startupDelay + rand.N(startupDelay))

	l, err := lru.New[uint64, string](totalLruSize / threadCount)
	if err != nil {
		log.Fatal(err)
	}
	rateLimiter := rate.NewLimiter(rate.Limit(totalAddRate/threadCount), addRateBurst)

	for i := uint64(0); ; i++ {
		beforeTime := time.Now()
		l.Add(i, fmt.Sprintf("val%d", i))
		elapsed := time.Since(beforeTime)

		if elapsed > pauseReportThreshold {
			log.Printf("lru add latency %v ms", elapsed.Milliseconds())
		}

		if err := rateLimiter.Wait(context.Background()); err != nil {
			panic(err)
		}
	}
}

// hiccupDetector measures how late a short sleep wakes up, as a proxy for
// stop-the-world pauses and scheduler stalls that affect every goroutine. The
// logged value is the oversleep beyond hiccupDetectorSleepDuration, truncated
// to whole milliseconds.
func hiccupDetector() {
	for {
		beforeTime := time.Now()
		time.Sleep(hiccupDetectorSleepDuration)
		slept := time.Since(beforeTime)
		if slept > hiccupDetectorThreshold {
			log.Printf("hiccup: %vms", (slept - hiccupDetectorSleepDuration).Milliseconds())
		}
	}
}
