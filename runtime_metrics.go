package sightpane

import (
	"runtime"
	"time"
)

// RuntimeMetrics captures Go runtime memory stats, goroutine counts, and GC activity.
type RuntimeMetrics struct {
	Timestamp         time.Time `json:"timestamp"`
	Goroutines        int       `json:"goroutines"`
	NumCPU            int       `json:"num_cpu"`
	CgoCalls          int64     `json:"cgo_calls"`
	AllocBytes        uint64    `json:"alloc_bytes"`
	TotalAllocBytes   uint64    `json:"total_alloc_bytes"`
	SysBytes          uint64    `json:"sys_bytes"`
	Lookups           uint64    `json:"lookups"`
	Mallocs           uint64    `json:"mallocs"`
	Frees             uint64    `json:"frees"`
	HeapAllocBytes    uint64    `json:"heap_alloc_bytes"`
	HeapSysBytes      uint64    `json:"heap_sys_bytes"`
	HeapIdleBytes     uint64    `json:"heap_idle_bytes"`
	HeapInuseBytes    uint64    `json:"heap_inuse_bytes"`
	HeapReleasedBytes uint64    `json:"heap_released_bytes"`
	HeapObjects       uint64    `json:"heap_objects"`
	StackInuseBytes   uint64    `json:"stack_inuse_bytes"`
	StackSysBytes     uint64    `json:"stack_sys_bytes"`
	MSpanInuseBytes   uint64    `json:"mspan_inuse_bytes"`
	MCacheInuseBytes  uint64    `json:"mcache_inuse_bytes"`
	NumGC             uint32    `json:"num_gc"`
	PauseTotalNs      uint64    `json:"pause_total_ns"`
	GCCPUFraction     float64   `json:"gc_cpu_fraction"`
}

// ReadRuntimeMetrics gathers current runtime metrics from the Go runtime.
func ReadRuntimeMetrics() RuntimeMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return RuntimeMetrics{
		Timestamp:         time.Now().UTC(),
		Goroutines:        runtime.NumGoroutine(),
		NumCPU:            runtime.NumCPU(),
		CgoCalls:          runtime.NumCgoCall(),
		AllocBytes:        m.Alloc,
		TotalAllocBytes:   m.TotalAlloc,
		SysBytes:          m.Sys,
		Lookups:           m.Lookups,
		Mallocs:           m.Mallocs,
		Frees:             m.Frees,
		HeapAllocBytes:    m.HeapAlloc,
		HeapSysBytes:      m.HeapSys,
		HeapIdleBytes:     m.HeapIdle,
		HeapInuseBytes:    m.HeapInuse,
		HeapReleasedBytes: m.HeapReleased,
		HeapObjects:       m.HeapObjects,
		StackInuseBytes:   m.StackInuse,
		StackSysBytes:     m.StackSys,
		MSpanInuseBytes:   m.MSpanInuse,
		MCacheInuseBytes:  m.MCacheInuse,
		NumGC:             m.NumGC,
		PauseTotalNs:      m.PauseTotalNs,
		GCCPUFraction:     m.GCCPUFraction,
	}
}

// ToProps converts the metrics struct into a property map for event ingestion.
func (rm RuntimeMetrics) ToProps() map[string]any {
	return map[string]any{
		"goroutines":          rm.Goroutines,
		"num_cpu":             rm.NumCPU,
		"cgo_calls":           rm.CgoCalls,
		"alloc_bytes":         rm.AllocBytes,
		"total_alloc_bytes":   rm.TotalAllocBytes,
		"sys_bytes":           rm.SysBytes,
		"lookups":             rm.Lookups,
		"mallocs":             rm.Mallocs,
		"frees":               rm.Frees,
		"heap_alloc_bytes":    rm.HeapAllocBytes,
		"heap_sys_bytes":      rm.HeapSysBytes,
		"heap_idle_bytes":     rm.HeapIdleBytes,
		"heap_inuse_bytes":    rm.HeapInuseBytes,
		"heap_released_bytes": rm.HeapReleasedBytes,
		"heap_objects":        rm.HeapObjects,
		"stack_inuse_bytes":   rm.StackInuseBytes,
		"stack_sys_bytes":     rm.StackSysBytes,
		"mspan_inuse_bytes":   rm.MSpanInuseBytes,
		"mcache_inuse_bytes":  rm.MCacheInuseBytes,
		"num_gc":              rm.NumGC,
		"pause_total_ns":      rm.PauseTotalNs,
		"gc_cpu_fraction":     rm.GCCPUFraction,
	}
}
