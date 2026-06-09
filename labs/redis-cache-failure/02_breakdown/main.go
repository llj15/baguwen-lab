package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ---------- 模拟数据库 ----------

var dbHitCount int64

func queryHotData(ctx context.Context) string {
	atomic.AddInt64(&dbHitCount, 1)
	time.Sleep(50 * time.Millisecond) // 模拟慢查询（聚合/JOIN等）
	return "微博热搜: #Redis面试题#"
}

// ---------- 无保护的查询（会被击穿） ----------

func getHotDataNoProtection(ctx context.Context, rdb *redis.Client) string {
	key := "hot:trending"

	// 1. 查缓存
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}

	// 2. 缓存miss → 直接查DB（所有并发请求都会走到这里！）
	data := queryHotData(ctx)
	rdb.Set(ctx, key, data, 3*time.Second) // 设置3秒过期，方便演示
	return data
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()

	rdb.FlushAll(ctx)

	fmt.Println("===== 缓存击穿演示 =====")
	fmt.Println()

	// ---------- 先预热缓存 ----------
	fmt.Println("--- 预热: 先写入热点缓存, TTL=3秒 ---")
	rdb.Set(ctx, "hot:trending", "微博热搜: #Redis面试题#", 3*time.Second)
	fmt.Println("  缓存已写入, 3秒后过期")
	fmt.Println()

	// ---------- 等待缓存过期 ----------
	fmt.Println("--- 等待缓存过期... ---")
	time.Sleep(4 * time.Second)
	fmt.Println("  缓存已过期!")
	fmt.Println()

	// ---------- 模拟100个并发请求同时到达 ----------
	fmt.Println("--- 100个并发请求同时查询(无保护) ---")
	atomic.StoreInt64(&dbHitCount, 0)
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			getHotDataNoProtection(ctx, rdb)
		}()
	}
	wg.Wait()

	elapsed := time.Since(start)
	fmt.Printf("  数据库被访问次数: %d (理想值=1, 实际全部穿透!)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Printf("  总耗时: %v\n", elapsed)
	fmt.Println()

	fmt.Println("===== 结论 =====")
	fmt.Println("热点key过期瞬间, 所有并发请求同时发现缓存miss,")
	fmt.Println("全部涌向数据库查同一条数据, 造成瞬间压力激增。")
	fmt.Println()

	// ========== 防御方案: singleflight ==========
	rdb.FlushAll(ctx)
	fmt.Println("===== 防御方案: singleflight =====")
	fmt.Println()

	fmt.Println("--- 100个并发请求同时查询(有保护) ---")
	atomic.StoreInt64(&dbHitCount, 0)
	start = time.Now()

	var wg2 sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			getHotDataWithProtection(ctx, rdb)
		}()
	}
	wg2.Wait()

	elapsed = time.Since(start)
	fmt.Printf("  数据库被访问次数: %d (singleflight合并为1次!)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Printf("  总耗时: %v\n", elapsed)
	fmt.Println()

	// ========== 防御方案2: Redis分布式锁 ==========
	rdb.FlushAll(ctx)
	fmt.Println("===== 防御方案: Redis分布式锁 (SET NX EX) =====")
	fmt.Println()

	fmt.Println("--- 100个并发请求同时查询(分布式锁) ---")
	atomic.StoreInt64(&dbHitCount, 0)
	start = time.Now()

	var wg3 sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg3.Add(1)
		go func() {
			defer wg3.Done()
			getHotDataWithDistLock(ctx, rdb)
		}()
	}
	wg3.Wait()

	elapsed = time.Since(start)
	fmt.Printf("  数据库被访问次数: %d (只有抢到锁的1个请求查DB)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Printf("  总耗时: %v\n", elapsed)
	fmt.Println()

	fmt.Println("===== 三种方案对比 =====")
	fmt.Println("  无保护:       100个请求全部打到DB")
	fmt.Println("  singleflight: 进程内合并, 1次DB查询, 最轻量")
	fmt.Println("  分布式锁:     跨进程互斥, 1次DB查询, 适合多实例部署")
}
