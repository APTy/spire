package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type benchmarkstate int

const (
	benchmarkstateInit benchmarkstate = iota
	benchmarkstateWarmup
	benchmarkstateRunning
	benchmarkstateFinished
)

type benchmarker struct {
	sync.Mutex

	TestDuration time.Duration
	RPS          int

	numRequests int
	state       benchmarkstate
	startTime   time.Time
	finishTime  time.Time
}

func (bm *benchmarker) Duration() int64 {
	return bm.finishTime.Unix() - bm.startTime.Unix()
}

func (bm *benchmarker) NumRequests() int {
	return bm.numRequests
}

func (bm *benchmarker) IncRequests() {
	if bm.state == benchmarkstateWarmup {
		return
	}
	bm.numRequests += 1
}

func (bm *benchmarker) TargetRPS() int {
	bm.Lock()
	defer bm.Unlock()
	if bm.state == benchmarkstateWarmup {
		return bm.RPS / 4
	}
	return bm.RPS
}

func (bm *benchmarker) Warmup() {
	bm.Lock()
	defer bm.Unlock()
	bm.state = benchmarkstateWarmup
	fmt.Println("Warming up...")
}

func (bm *benchmarker) Run() {
	// wait for warmup to complete
	<-time.After(bm.TestDuration / 4)

	bm.Lock()
	bm.state = benchmarkstateRunning
	bm.startTime = time.Now()
	fmt.Println("Running...")
	bm.Unlock()

	// wait for run to complete
	<-time.After(bm.TestDuration)

	bm.Lock()
	bm.state = benchmarkstateFinished
	bm.finishTime = time.Now()
	fmt.Println("Done.")
	bm.Unlock()
}

func doBenchmark(testRPS int, testDuration time.Duration, benchmarkFunction func() error) {
	var wg sync.WaitGroup
	done := make(chan struct{})
	requestQueue := make(chan struct{}, testRPS)
	metricsQueue := make(chan metric, testRPS*1000)

	metrics := make([]metric, 0, testRPS*1000)
	bm := benchmarker{
		RPS:          testRPS,
		TestDuration: testDuration,
	}
	bm.Warmup()

	// Queue up n requests every second
	go func() {
		splayOverOneSecond := 10
		for {
			for i := 0; i < bm.TargetRPS()/splayOverOneSecond; i++ {
				wg.Add(1)
				requestQueue <- struct{}{}
			}
			select {
			case <-done:
				return
			case <-time.After(time.Duration(1000/splayOverOneSecond) * time.Millisecond):
			}
		}
	}()

	// Continuously pull off the metrics output queue
	go func() {
		for m := range metricsQueue {
			bm.IncRequests()
			metrics = append(metrics, m)
			wg.Done()
		}
	}()

	// Make parallel calls and track their duration
	go func() {
		for _ = range requestQueue {
			go func() {
				startTime := time.Now()
				err := benchmarkFunction()
				endTime := time.Now()
				m := metric{
					Duration: endTime.Sub(startTime),
				}
				if err != nil {
					m.Err = err
				}

				metricsQueue <- m
			}()
		}
	}()

	bm.Run()
	close(done)
	wg.Wait()

	sort.Sort(byDuration(metrics))
	percentile := func(n int) float64 {
		i := bm.NumRequests() * n / 100
		return float64(metrics[i].Duration.Nanoseconds()) / 1e6
	}

	fmt.Printf("Completed %d requests in %d seconds:\n", bm.NumRequests(), bm.Duration())
	fmt.Printf("  rps: %d\n", bm.NumRequests()/int(bm.Duration()))
	fmt.Printf("  p50: %.0fms\n", percentile(50))
	fmt.Printf("  p95: %.0fms\n", percentile(95))
	fmt.Printf("  p99: %.0fms\n", percentile(99))
	fmt.Printf("  errors: %.2f%%\n", 100.0*errCount(metrics)/float64(bm.NumRequests()))
}
