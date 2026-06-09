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

func queryProduct(id int) string {
	atomic.AddInt64(&dbHitCount, 1)
	time.Sleep(10 * time.Millisecond) // 模拟查询耗时
	return fmt.Sprintf("商品_%d_信息", id)
}

// ---------- 批量预热：所有key设相同TTL（会雪崩） ----------

func warmUpSameTTL(ctx context.Context, rdb *redis.Client, count int) {
	for i := 1; i <= count; i++ {
		key := fmt.Sprintf("product:%d", i)
		rdb.Set(ctx, key, fmt.Sprintf("商品_%d_信息", i), 3*time.Second) // 全部3秒过期
	}
	fmt.Printf("  预热完成: %d个key, 全部TTL=3秒\n", count)
}

// ---------- 无保护查询 ----------

func getProduct(ctx context.Context, rdb *redis.Client, id int) string {
	key := fmt.Sprintf("product:%d", id)

	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}

	// 缓存miss → 查DB
	data := queryProduct(id)
	rdb.Set(ctx, key, data, 3*time.Second)
	return data
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()

	rdb.FlushAll(ctx)

	productCount := 200

	fmt.Println("===== 缓存雪崩演示 =====")
	fmt.Println()

	// ---------- 预热缓存 ----------
	fmt.Println("--- 预热: 200个商品缓存, 全部TTL=3秒 ---")
	warmUpSameTTL(ctx, rdb, productCount)

	// 验证缓存正常工作
	atomic.StoreInt64(&dbHitCount, 0)
	for i := 1; i <= productCount; i++ {
		getProduct(ctx, rdb, i)
	}
	fmt.Printf("  预热后查询200个商品, DB访问=%d (全部走缓存)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Println()

	// ---------- 等待全部过期 ----------
	fmt.Println("--- 等待3秒, 所有缓存同时过期... ---")
	time.Sleep(4 * time.Second)

	// 查看Redis里还有多少key
	keys, _ := rdb.Keys(ctx, "product:*").Result()
	fmt.Printf("  Redis中剩余product key数量: %d (全部过期!)\n", len(keys))
	fmt.Println()

	// ---------- 雪崩：所有请求同时打到DB ----------
	fmt.Println("--- 雪崩! 200个并发请求同时到达 ---")
	atomic.StoreInt64(&dbHitCount, 0)
	start := time.Now()

	var wg sync.WaitGroup
	for i := 1; i <= productCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			getProduct(ctx, rdb, id)
		}(i)
	}
	wg.Wait()

	elapsed := time.Since(start)
	fmt.Printf("  数据库被访问次数: %d (全部穿透! 数据库瞬间压力拉满)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Printf("  总耗时: %v\n", elapsed)
	fmt.Println()

	fmt.Println("===== 结论 =====")
	fmt.Println("所有key设置相同的TTL → 同时过期 → 请求全部涌向数据库")
	fmt.Println("这就是缓存雪崩, 数据库可能直接被打宕机。")
}
