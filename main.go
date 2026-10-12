package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

type Network struct {
	IP   string
	Port string
}

type Status struct {
	RAM  float64 `json:"ram_usage_percent"`
	Disk float64 `json:"disk_usage_percent"`
	CPU  float64 `json:"cpu_usage_percent"`
}

func main() {
	network := Network{
		IP:   "0.0.0.0",
		Port: "8000",
	}

	http.HandleFunc("/health", healthCheckHandler)
	http.HandleFunc("/", healthCheckHandler)

	addr := fmt.Sprintf("%s:%s", network.IP, network.Port)
	log.Printf("Server is online: http://%s\n", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := &Status{}
	diskPath := "/"

	if err := status.GetCurrentSnapshot(ctx, diskPath); err != nil {
		http.Error(w, fmt.Sprintf("Failed to get system stats: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(status); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func GetCPUUsage(ctx context.Context) (float64, error) {
	percentages, err := cpu.PercentWithContext(ctx, 200*time.Millisecond, false)
	if err != nil {
		return -1, fmt.Errorf("failed to fetch cpu usage: %w", err)
	}

	var usage float64
	if len(percentages) > 0 {
		usage = percentages[0]
	}

	return usage, nil
}

func GetMemoryUsage(ctx context.Context) (float64, error) {
	vMem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return -1, fmt.Errorf("failed to fetch memory usage: %w", err)
	}

	return vMem.UsedPercent, nil
}

func GetDiskUsage(ctx context.Context, mountPath string) (float64, error) {
	dUsage, err := disk.UsageWithContext(ctx, mountPath)
	if err != nil {
		return -1, fmt.Errorf("failed to fetch disk usage for %s: %w", mountPath, err)
	}

	return dUsage.UsedPercent, nil
}

func (s *Status) GetCurrentSnapshot(ctx context.Context, diskPath string) error {
	var (
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)

	wg.Add(3)

	// CPU
	go func() {
		defer wg.Done()
		cpuStats, err := GetCPUUsage(ctx)
		if err != nil {
			errOnce.Do(func() { firstErr = err })
			return
		}
		s.CPU = cpuStats
	}()

	// RAM
	go func() {
		defer wg.Done()
		memStats, err := GetMemoryUsage(ctx)
		if err != nil {
			errOnce.Do(func() { firstErr = err })
			return
		}
		s.RAM = memStats
	}()

	// Disk
	go func() {
		defer wg.Done()
		diskStats, err := GetDiskUsage(ctx, diskPath)
		if err != nil {
			errOnce.Do(func() { firstErr = err })
			return
		}
		s.Disk = diskStats
	}()

	wg.Wait()

	if firstErr != nil {
		return firstErr
	}

	return nil
}
