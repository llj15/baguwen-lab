package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

func redisAddrs() []string {
	addrs := []string{}
	for _, env := range []string{"REDIS_ADDR_1", "REDIS_ADDR_2", "REDIS_ADDR_3"} {
		if addr := os.Getenv(env); addr != "" {
			addrs = append(addrs, addr)
		}
	}
	if len(addrs) == 0 {
		return []string{"localhost:6379", "localhost:6380", "localhost:6381"}
	}
	return addrs
}

func newClients(addrs []string) []*redis.Client {
	clients := make([]*redis.Client, len(addrs))
	for i, addr := range addrs {
		clients[i] = redis.NewClient(&redis.Options{Addr: addr})
	}
	return clients
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

func randomValue() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ==================== Redlock Implementation ====================

type RedlockResult struct {
	Acquired   bool
	Value      string
	ValidUntil time.Time
}

// Try to acquire lock on a single Redis instance
func acquireSingle(client *redis.Client, key, value string, ttl time.Duration) bool {
	ok, err := client.SetNX(ctx, key, value, ttl).Result()
	return err == nil && ok
}

// Release lock on a single Redis instance (only if value matches)
func releaseSingle(client *redis.Client, key, value string) {
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		end
		return 0
	`)
	script.Run(ctx, client, []string{key}, value)
}

// Redlock: acquire lock across N/2+1 instances
func redlockAcquire(clients []*redis.Client, key string, ttl time.Duration) RedlockResult {
	value := randomValue()
	quorum := len(clients)/2 + 1
	start := time.Now()

	acquired := 0
	for _, client := range clients {
		if acquireSingle(client, key, value, ttl) {
			acquired++
		}
	}

	elapsed := time.Since(start)
	// Valid time = TTL - time spent acquiring
	validTime := ttl - elapsed
	// Clock drift compensation (small factor)
	drift := time.Duration(float64(ttl) * 0.01)
	validTime -= drift

	if acquired >= quorum && validTime > 0 {
		return RedlockResult{
			Acquired:   true,
			Value:      value,
			ValidUntil: time.Now().Add(validTime),
		}
	}

	// Failed: release all locks
	for _, client := range clients {
		releaseSingle(client, key, value)
	}
	return RedlockResult{Acquired: false}
}

func redlockRelease(clients []*redis.Client, key, value string) {
	for _, client := range clients {
		releaseSingle(client, key, value)
	}
}

// ==================== Experiment 2a: Redlock correctness ====================

func testRedlock(workers, increments int) map[string]interface{} {
	addrs := redisAddrs()
	clients := newClients(addrs)
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()

	// Use first Redis instance for the counter
	rdb := clients[0]
	rdb.Set(ctx, "counter:redlock", 0, 0)

	var wg sync.WaitGroup
	var retries atomic.Int64
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			myClients := newClients(addrs)
			defer func() {
				for _, c := range myClients {
					c.Close()
				}
			}()

			for j := 0; j < increments; j++ {
				var result RedlockResult
				for {
					result = redlockAcquire(myClients, "redlock:counter", 5*time.Second)
					if result.Acquired {
						break
					}
					retries.Add(1)
					time.Sleep(5 * time.Millisecond)
				}

				// Critical section
				val, _ := rdb.Get(ctx, "counter:redlock").Int()
				rdb.Set(ctx, "counter:redlock", val+1, 0)

				redlockRelease(myClients, "redlock:counter", result.Value)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	finalVal, _ := rdb.Get(ctx, "counter:redlock").Int()
	expected := workers * increments
	lost := expected - finalVal

	fmt.Printf("[Redlock 3-node] expected=%d actual=%d lost=%d retries=%d duration=%v\n",
		expected, finalVal, lost, retries.Load(), elapsed)

	return map[string]interface{}{
		"mode":        "redlock_3_node",
		"nodes":       len(addrs),
		"quorum":      len(addrs)/2 + 1,
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

// ==================== Experiment 2b: Redlock with one node down ====================

func testRedlockWithFailure(workers, increments int) map[string]interface{} {
	addrs := redisAddrs()

	// Simulate node failure: only use first 2 nodes (quorum still met with 2/3)
	failedAddrs := addrs[:2]
	clients := newClients(failedAddrs)
	// But we still check all 3 for quorum logic - add a dead one
	deadClient := redis.NewClient(&redis.Options{
		Addr:        "dead-node:6379",
		DialTimeout: 100 * time.Millisecond,
	})
	allClients := append(clients, deadClient)
	defer func() {
		for _, c := range allClients {
			c.Close()
		}
	}()

	// Counter on first node
	rdb := clients[0]
	rdb.Set(ctx, "counter:redlock_fail", 0, 0)

	var wg sync.WaitGroup
	var retries atomic.Int64
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			myAlive := newClients(failedAddrs)
			myDead := redis.NewClient(&redis.Options{
				Addr:        "dead-node:6379",
				DialTimeout: 100 * time.Millisecond,
			})
			myAll := append(myAlive, myDead)
			defer func() {
				for _, c := range myAll {
					c.Close()
				}
			}()

			for j := 0; j < increments; j++ {
				var result RedlockResult
				for {
					result = redlockAcquire(myAll, "redlock:counter_fail", 5*time.Second)
					if result.Acquired {
						break
					}
					retries.Add(1)
					time.Sleep(5 * time.Millisecond)
				}

				val, _ := rdb.Get(ctx, "counter:redlock_fail").Int()
				rdb.Set(ctx, "counter:redlock_fail", val+1, 0)

				redlockRelease(myAll, "redlock:counter_fail", result.Value)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	finalVal, _ := rdb.Get(ctx, "counter:redlock_fail").Int()
	expected := workers * increments
	lost := expected - finalVal

	fmt.Printf("[Redlock 1-node-down] expected=%d actual=%d lost=%d retries=%d duration=%v\n",
		expected, finalVal, lost, retries.Load(), elapsed)

	return map[string]interface{}{
		"mode":          "redlock_1_node_down",
		"total_nodes":   3,
		"alive_nodes":   2,
		"quorum":        2,
		"workers":       workers,
		"increments":    increments,
		"expected":      expected,
		"actual":        finalVal,
		"lost":          lost,
		"lost_pct":      float64(lost) / float64(expected) * 100,
		"retries":       retries.Load(),
		"duration_ms":   elapsed.Milliseconds(),
	}
}

func main() {
	fmt.Println("=== Experiment 2: Redlock Algorithm ===")
	fmt.Println()

	fmt.Println("--- 2a: Redlock with 3 healthy nodes ---")
	healthy := testRedlock(10, 50)

	fmt.Println("\n--- 2b: Redlock with 1 node down (2/3 quorum) ---")
	failed := testRedlockWithFailure(10, 50)

	results := map[string]interface{}{
		"results": []interface{}{healthy, failed},
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	path := outputPath("02_redlock.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("Warning: could not write JSON: %v\n", err)
	}
	fmt.Printf("\nJSON results written to %s\n", path)
}
