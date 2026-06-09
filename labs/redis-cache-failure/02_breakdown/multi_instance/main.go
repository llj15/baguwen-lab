package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

var dbHitCount int64

func queryHotData() string {
	atomic.AddInt64(&dbHitCount, 1)
	time.Sleep(50 * time.Millisecond)
	return "微博热搜: #Redis面试题#"
}

// ---------- 方案1: singleflight (进程内合并) ----------

var sfGroup singleflight.Group

func getWithSingleflight(ctx context.Context, rdb *redis.Client) string {
	key := "hot:trending"

	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}

	result, _, _ := sfGroup.Do(key, func() (interface{}, error) {
		data := queryHotData()
		rdb.Set(ctx, key, data, 10*time.Second)
		return data, nil
	})
	return result.(string)
}

// ---------- 方案2: Redis分布式锁 (跨进程互斥) ----------

func getWithDistLock(ctx context.Context, rdb *redis.Client) string {
	key := "hot:trending"
	lockKey := "lock:" + key

	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}

	ok, _ := rdb.SetNX(ctx, lockKey, "1", 5*time.Second).Result()
	if ok {
		defer rdb.Del(ctx, lockKey)
		data := queryHotData()
		rdb.Set(ctx, key, data, 10*time.Second)
		return data
	}

	for i := 0; i < 20; i++ {
		time.Sleep(10 * time.Millisecond)
		val, err = rdb.Get(ctx, key).Result()
		if err == nil {
			return val
		}
	}
	return queryHotData()
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("用法: ./multi_instance <instance_id> <mode>")
		fmt.Println("  mode: singleflight | distlock")
		os.Exit(1)
	}

	instanceID := os.Args[1]
	mode := os.Args[2]

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()

	// 每个实例内部启动20个并发请求
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch mode {
			case "singleflight":
				getWithSingleflight(ctx, rdb)
			case "distlock":
				getWithDistLock(ctx, rdb)
			}
		}()
	}
	wg.Wait()

	elapsed := time.Since(start)
	fmt.Printf("  实例[%s] mode=%s  DB访问=%d  耗时=%v\n",
		instanceID, mode, atomic.LoadInt64(&dbHitCount), elapsed)
}
