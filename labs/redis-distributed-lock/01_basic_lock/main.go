package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR_1"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: redisAddr()})
}

func outputPath(filename string) string {
	dir := os.Getenv("OUTPUT_DIR")
	if dir == "" {
		dir = "/data"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Warning: could not create output directory %s: %v\n", dir, err)
	}
	return filepath.Join(dir, filename)
}

// ==================== Experiment 1a: No Lock (Race Condition) ====================

func testNoLock(workers int, increments int) map[string]interface{} {
	rdb := newClient()
	defer rdb.Close()
	rdb.Set(ctx, "counter:nolock", 0, 0)

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				// Read-modify-write WITHOUT lock = race condition
				val, _ := rdb.Get(ctx, "counter:nolock").Int()
				rdb.Set(ctx, "counter:nolock", val+1, 0)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	finalVal, _ := rdb.Get(ctx, "counter:nolock").Int()
	expected := workers * increments
	lost := expected - finalVal

	fmt.Printf("[No Lock] expected=%d actual=%d lost=%d (%.1f%%) duration=%v\n",
		expected, finalVal, lost, float64(lost)/float64(expected)*100, elapsed)

	return map[string]interface{}{
		"mode":        "no_lock",
		"workers":     workers,
		"increments":  increments,
		"expected":    expected,
		"actual":      finalVal,
		"lost":        lost,
		"lost_pct":    float64(lost) / float64(expected) * 100,
		"duration_ms": elapsed.Milliseconds(),
	}
}

// ==================== Experiment 1b: SET NX EX Lock ====================

func acquireLock(rdb *redis.Client, key, value string, ttl time.Duration) bool {
	ok, err := rdb.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false
	}
	return ok
}

func releaseLock(rdb *redis.Client, key, value string) bool {
	// Lua script: only delete if value matches (prevent deleting others' lock)
	// GET + DEL must be atomic, otherwise race condition between check and delete
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		end
		return 0
	`)
	result, err := script.Run(ctx, rdb, []string{key}, value).Int()
	if err != nil {
		return false
	}
	return result == 1
}

func testBasicLock(workers int, increments int, lockTTL time.Duration) map[string]interface{} {
	rdb := newClient()
	defer rdb.Close()
	rdb.Set(ctx, "counter:locked", 0, 0)
	rdb.Del(ctx, "lock:counter")

	var wg sync.WaitGroup
	var retries atomic.Int64
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lockVal := fmt.Sprintf("worker-%d", id)
			for j := 0; j < increments; j++ {
				// Spin to acquire lock
				for {
					if acquireLock(rdb, "lock:counter", lockVal, lockTTL) {
						break
					}
					retries.Add(1)
					time.Sleep(1 * time.Millisecond)
				}

				// Critical section: read-modify-write WITH lock
				val, _ := rdb.Get(ctx, "counter:locked").Int()
				rdb.Set(ctx, "counter:locked", val+1, 0)

				releaseLock(rdb, "lock:counter", lockVal)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	finalVal, _ := rdb.Get(ctx, "counter:locked").Int()
	expected := workers * increments
	lost := expected - finalVal

	fmt.Printf("[SET NX EX] expected=%d actual=%d lost=%d retries=%d duration=%v\n",
		expected, finalVal, lost, retries.Load(), elapsed)

	return map[string]interface{}{
		"mode":        "set_nx_ex",
		"workers":     workers,
		"increments":  increments,
		"expected":    expected,
		"actual":      finalVal,
		"lost":        lost,
		"lost_pct":    float64(lost) / float64(expected) * 100,
		"retries":     retries.Load(),
		"duration_ms": elapsed.Milliseconds(),
	}
}

// ==================== Experiment 1c: Lock TTL too short (unsafe) ====================

func testShortTTLLock(workers int, increments int) map[string]interface{} {
	rdb := newClient()
	defer rdb.Close()
	rdb.Set(ctx, "counter:shortttl", 0, 0)
	rdb.Del(ctx, "lock:shortttl")

	var wg sync.WaitGroup
	var wrongRelease atomic.Int64
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lockVal := fmt.Sprintf("worker-%d", id)
			for j := 0; j < increments; j++ {
				for {
					if acquireLock(rdb, "lock:shortttl", lockVal, 5*time.Millisecond) {
						break
					}
					time.Sleep(1 * time.Millisecond)
				}

				// Simulate slow business logic that exceeds lock TTL
				time.Sleep(10 * time.Millisecond)

				val, _ := rdb.Get(ctx, "counter:shortttl").Int()
				rdb.Set(ctx, "counter:shortttl", val+1, 0)

				if !releaseLock(rdb, "lock:shortttl", lockVal) {
					wrongRelease.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	finalVal, _ := rdb.Get(ctx, "counter:shortttl").Int()
	expected := workers * increments
	lost := expected - finalVal

	fmt.Printf("[Short TTL] expected=%d actual=%d lost=%d wrong_release=%d duration=%v\n",
		expected, finalVal, lost, wrongRelease.Load(), elapsed)

	return map[string]interface{}{
		"mode":            "short_ttl",
		"workers":         workers,
		"increments":      increments,
		"expected":        expected,
		"actual":          finalVal,
		"lost":            lost,
		"lost_pct":        float64(lost) / float64(expected) * 100,
		"wrong_release":   wrongRelease.Load(),
		"duration_ms":     elapsed.Milliseconds(),
	}
}

func main() {
	fmt.Println("=== Experiment 1: Basic Distributed Lock ===")
	fmt.Println()

	workers := 10
	increments := 100

	fmt.Println("--- 1a: No Lock (demonstrates race condition) ---")
	noLock := testNoLock(workers, increments)

	fmt.Println("\n--- 1b: SET NX EX Lock (correct implementation) ---")
	withLock := testBasicLock(workers, increments, 5*time.Second)

	fmt.Println("\n--- 1c: Short TTL Lock (TTL < business time, unsafe) ---")
	shortTTL := testShortTTLLock(5, 20) // fewer ops since each takes 10ms+

	results := map[string]interface{}{
		"results": []interface{}{noLock, withLock, shortTTL},
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	path := outputPath("01_basic_lock.json")
	os.WriteFile(path, data, 0644)
	fmt.Printf("\nJSON results written to %s\n", path)
}
