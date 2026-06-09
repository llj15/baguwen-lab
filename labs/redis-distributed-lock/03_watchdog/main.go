package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var bgCtx = context.Background()

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
		log.Printf("Warning: could not create output directory %s: %v", dir, err)
	}
	return filepath.Join(dir, filename)
}

// ==================== Watchdog Lock Implementation ====================

type WatchdogLock struct {
	rdb      *redis.Client
	key      string
	value    string
	ttl      time.Duration
	cancel   context.CancelFunc
	renewals atomic.Int64
}

func NewWatchdogLock(rdb *redis.Client, key, value string, ttl time.Duration) *WatchdogLock {
	return &WatchdogLock{
		rdb:   rdb,
		key:   key,
		value: value,
		ttl:   ttl,
	}
}

func (w *WatchdogLock) Lock() bool {
	ok, err := w.rdb.SetNX(bgCtx, w.key, w.value, w.ttl).Result()
	if err != nil || !ok {
		return false
	}

	// Start watchdog goroutine: renew at ttl/3 intervals
	ctx, cancel := context.WithCancel(bgCtx)
	w.cancel = cancel

	go func() {
		ticker := time.NewTicker(w.ttl / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Only renew if we still own the lock
				renewed := w.renew()
				if !renewed {
					return
				}
				w.renewals.Add(1)
			}
		}
	}()

	return true
}

func (w *WatchdogLock) renew() bool {
	// Lua: only extend TTL if value still matches
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		end
		return 0
	`)
	result, err := script.Run(bgCtx, w.rdb, []string{w.key}, w.value, w.ttl.Milliseconds()).Int()
	return err == nil && result == 1
}

func (w *WatchdogLock) Unlock() bool {
	// Stop watchdog first
	if w.cancel != nil {
		w.cancel()
	}

	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		end
		return 0
	`)
	result, err := script.Run(bgCtx, w.rdb, []string{w.key}, w.value).Int()
	return err == nil && result == 1
}

func (w *WatchdogLock) Renewals() int64 {
	return w.renewals.Load()
}

// ==================== Experiment 3a: Without watchdog (lock expires during work) ====================

func testNoWatchdog() map[string]interface{} {
	rdb := newClient()
	defer rdb.Close()
	rdb.Set(bgCtx, "counter:no_watchdog", 0, 0)
	rdb.Del(bgCtx, "lock:no_watchdog")

	lockTTL := 500 * time.Millisecond // Short TTL
	workDuration := 800 * time.Millisecond // Work takes longer than TTL!
	workers := 5
	increments := 5

	var wg sync.WaitGroup
	var violations atomic.Int64
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lockVal := fmt.Sprintf("worker-%d", id)
			for j := 0; j < increments; j++ {
				// Acquire lock with short TTL, no watchdog
				for {
					ok, _ := rdb.SetNX(bgCtx, "lock:no_watchdog", lockVal, lockTTL).Result()
					if ok {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}

				// Simulate long business logic (exceeds lock TTL)
				val, _ := rdb.Get(bgCtx, "counter:no_watchdog").Int()
				time.Sleep(workDuration)
				rdb.Set(bgCtx, "counter:no_watchdog", val+1, 0)

				// Try to release - may fail because lock already expired
				script := redis.NewScript(`
					if redis.call("GET", KEYS[1]) == ARGV[1] then
						return redis.call("DEL", KEYS[1])
					end
					return 0
				`)
				result, _ := script.Run(bgCtx, rdb, []string{"lock:no_watchdog"}, lockVal).Int()
				if result == 0 {
					violations.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	finalVal, _ := rdb.Get(bgCtx, "counter:no_watchdog").Int()
	expected := workers * increments
	lost := expected - finalVal

	fmt.Printf("[No Watchdog] expected=%d actual=%d lost=%d violations=%d duration=%v\n",
		expected, finalVal, lost, violations.Load(), elapsed)

	return map[string]interface{}{
		"mode":         "no_watchdog",
		"lock_ttl_ms":  lockTTL.Milliseconds(),
		"work_time_ms": workDuration.Milliseconds(),
		"workers":      workers,
		"increments":   increments,
		"expected":     expected,
		"actual":       finalVal,
		"lost":         lost,
		"lost_pct":     float64(lost) / float64(expected) * 100,
		"violations":   violations.Load(),
		"duration_ms":  elapsed.Milliseconds(),
	}
}

// ==================== Experiment 3b: With watchdog (auto-renewal keeps lock alive) ====================

func testWithWatchdog() map[string]interface{} {
	rdb := newClient()
	defer rdb.Close()
	rdb.Set(bgCtx, "counter:watchdog", 0, 0)
	rdb.Del(bgCtx, "lock:watchdog")

	lockTTL := 500 * time.Millisecond // Same short TTL
	workDuration := 800 * time.Millisecond // Same long work
	workers := 5
	increments := 5

	var wg sync.WaitGroup
	var totalRenewals atomic.Int64
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lockVal := fmt.Sprintf("worker-%d", id)
			for j := 0; j < increments; j++ {
				lock := NewWatchdogLock(rdb, "lock:watchdog", lockVal, lockTTL)
				for {
					if lock.Lock() {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}

				// Same long business logic - but watchdog keeps the lock alive!
				val, _ := rdb.Get(bgCtx, "counter:watchdog").Int()
				time.Sleep(workDuration)
				rdb.Set(bgCtx, "counter:watchdog", val+1, 0)

				totalRenewals.Add(lock.Renewals())
				lock.Unlock()
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	finalVal, _ := rdb.Get(bgCtx, "counter:watchdog").Int()
	expected := workers * increments
	lost := expected - finalVal

	fmt.Printf("[With Watchdog] expected=%d actual=%d lost=%d renewals=%d duration=%v\n",
		expected, finalVal, lost, totalRenewals.Load(), elapsed)

	return map[string]interface{}{
		"mode":           "with_watchdog",
		"lock_ttl_ms":    lockTTL.Milliseconds(),
		"work_time_ms":   workDuration.Milliseconds(),
		"renewal_interval": "ttl/3",
		"workers":        workers,
		"increments":     increments,
		"expected":       expected,
		"actual":         finalVal,
		"lost":           lost,
		"lost_pct":       float64(lost) / float64(expected) * 100,
		"total_renewals": totalRenewals.Load(),
		"duration_ms":    elapsed.Milliseconds(),
	}
}

// ==================== Experiment 3c: Watchdog renewal timeline ====================

func testWatchdogTimeline() map[string]interface{} {
	rdb := newClient()
	defer rdb.Close()
	rdb.Del(bgCtx, "lock:timeline")

	lockTTL := 300 * time.Millisecond
	workDuration := 2 * time.Second

	type Event struct {
		TimeMs int64  `json:"time_ms"`
		Event  string `json:"event"`
		TTLMs  int64  `json:"ttl_ms"`
	}

	var events []Event
	var mu sync.Mutex

	addEvent := func(start time.Time, event string, ttlMs int64) {
		mu.Lock()
		events = append(events, Event{
			TimeMs: time.Since(start).Milliseconds(),
			Event:  event,
			TTLMs:  ttlMs,
		})
		mu.Unlock()
	}

	start := time.Now()
	lock := NewWatchdogLock(rdb, "lock:timeline", "owner-1", lockTTL)

	if lock.Lock() {
		ttl, _ := rdb.PTTL(bgCtx, "lock:timeline").Result()
		addEvent(start, "lock_acquired", ttl.Milliseconds())

		// Monitor TTL changes during work
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					ttl, err := rdb.PTTL(bgCtx, "lock:timeline").Result()
					if err == nil && ttl > 0 {
						addEvent(start, "ttl_check", ttl.Milliseconds())
					}
				}
			}
		}()

		// Simulate work
		time.Sleep(workDuration)
		close(done)

		ttl, _ = rdb.PTTL(bgCtx, "lock:timeline").Result()
		addEvent(start, "before_unlock", ttl.Milliseconds())

		lock.Unlock()
		addEvent(start, "unlocked", 0)
	}

	fmt.Printf("[Watchdog Timeline] lock_ttl=%v work_duration=%v renewals=%d events=%d\n",
		lockTTL, workDuration, lock.Renewals(), len(events))

	return map[string]interface{}{
		"mode":          "watchdog_timeline",
		"lock_ttl_ms":   lockTTL.Milliseconds(),
		"work_time_ms":  workDuration.Milliseconds(),
		"renewals":      lock.Renewals(),
		"events":        events,
	}
}

func main() {
	fmt.Println("=== Experiment 3: Watchdog Auto-Renewal ===")
	fmt.Println()

	fmt.Println("--- 3a: No Watchdog (lock expires during work) ---")
	noWatchdog := testNoWatchdog()

	fmt.Println("\n--- 3b: With Watchdog (auto-renewal) ---")
	withWatchdog := testWithWatchdog()

	fmt.Println("\n--- 3c: Watchdog Renewal Timeline ---")
	timeline := testWatchdogTimeline()

	results := map[string]interface{}{
		"results": []interface{}{noWatchdog, withWatchdog, timeline},
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	path := outputPath("03_watchdog.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("Warning: could not write JSON: %v", err)
	}
	fmt.Printf("\nJSON results written to %s\n", path)
}
