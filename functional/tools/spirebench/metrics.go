package main

import "time"

type metric struct {
	Duration time.Duration
	Err      error
}

type byDuration []metric

func (m byDuration) Len() int {
	return len(m)
}

func (m byDuration) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

func (m byDuration) Less(i, j int) bool {
	return m[i].Duration < m[j].Duration
}

func errCount(metrics []metric) float64 {
	i := 0.0
	for _, m := range metrics {
		if m.Err != nil {
			i++
		}
	}
	return i
}
