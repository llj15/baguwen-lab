package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ---------- 模拟数据库 ----------

// 假装这是数据库，只有 ID 1~5 的用户存在
var fakeDB = map[int]string{
	1: "Alice",
	2: "Bob",
	3: "Charlie",
	4: "David",
	5: "Eve",
}

// 统计数据库被访问的次数
var dbHitCount int64

func queryDB(id int) (string, bool) {
	atomic.AddInt64(&dbHitCount, 1)
	time.Sleep(5 * time.Millisecond) // 模拟数据库查询耗时
	name, ok := fakeDB[id]
	return name, ok
}

// ---------- 无保护的查询（会被穿透） ----------

func getUserNoProtection(ctx context.Context, rdb *redis.Client, id int) string {
	key := fmt.Sprintf("user:%d", id)

	// 1. 先查缓存
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val // 缓存命中
	}

	// 2. 缓存未命中，查数据库
	name, ok := queryDB(id)
	if ok {
		// 数据库有数据，写入缓存
		rdb.Set(ctx, key, name, 5*time.Minute)
		return name
	}

	// 3. 数据库也没有 → 什么都不缓存 → 下次还会穿透！
	return ""
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()

	// 清空 Redis
	rdb.FlushAll(ctx)

	fmt.Println("===== 缓存穿透演示 =====")
	fmt.Println()

	// ---------- 场景1: 正常查询 ----------
	fmt.Println("--- 场景1: 查询存在的用户(ID=1), 连续5次 ---")
	atomic.StoreInt64(&dbHitCount, 0)

	for i := 0; i < 5; i++ {
		name := getUserNoProtection(ctx, rdb, 1)
		fmt.Printf("  第%d次查询: user=%s\n", i+1, name)
	}
	fmt.Printf("  数据库被访问次数: %d (理想值=1, 后4次走缓存)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Println()

	// ---------- 场景2: 穿透攻击 ----------
	fmt.Println("--- 场景2: 查询不存在的用户(ID=-1), 连续5次 ---")
	atomic.StoreInt64(&dbHitCount, 0)

	for i := 0; i < 5; i++ {
		name := getUserNoProtection(ctx, rdb, -1)
		fmt.Printf("  第%d次查询: user=%q\n", i+1, name)
	}
	fmt.Printf("  数据库被访问次数: %d (全部穿透! 每次都打到DB)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Println()

	// ---------- 场景3: 模拟大量恶意请求 ----------
	fmt.Println("--- 场景3: 1000个不存在的ID同时请求 ---")
	atomic.StoreInt64(&dbHitCount, 0)
	start := time.Now()

	for i := -1000; i < 0; i++ {
		getUserNoProtection(ctx, rdb, i)
	}
	elapsed := time.Since(start)
	fmt.Printf("  数据库被访问次数: %d\n", atomic.LoadInt64(&dbHitCount))
	fmt.Printf("  总耗时: %v (每次都走DB, 非常慢)\n", elapsed)
	fmt.Println()

	fmt.Println("===== 结论 =====")
	fmt.Println("不存在的key永远不会被缓存,")
	fmt.Println("攻击者用大量不存在的ID就能直接打穿缓存层, 压垮数据库。")
	fmt.Println()

	// ========== 防御方案: 缓存空值 ==========
	rdb.FlushAll(ctx)
	fmt.Println("===== 防御方案: 缓存空值 =====")
	fmt.Println()

	fmt.Println("--- 场景2(防御版): 查询不存在的用户(ID=-1), 连续5次 ---")
	atomic.StoreInt64(&dbHitCount, 0)

	for i := 0; i < 5; i++ {
		name := getUserWithProtection(ctx, rdb, -1)
		fmt.Printf("  第%d次查询: user=%q\n", i+1, name)
	}
	fmt.Printf("  数据库被访问次数: %d (只有第1次穿透, 后续命中空值缓存)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Println()

	fmt.Println("--- 场景3(防御版): 1000个不存在的ID ---")
	rdb.FlushAll(ctx)
	atomic.StoreInt64(&dbHitCount, 0)
	start = time.Now()

	for i := -1000; i < 0; i++ {
		getUserWithProtection(ctx, rdb, i)
	}
	elapsed = time.Since(start)
	fmt.Printf("  数据库被访问次数: %d (每个ID只穿透1次)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Printf("  总耗时: %v\n", elapsed)
	fmt.Println()

	fmt.Println("--- 再跑一遍同样的1000个ID (全部命中空值缓存) ---")
	atomic.StoreInt64(&dbHitCount, 0)
	start = time.Now()

	for i := -1000; i < 0; i++ {
		getUserWithProtection(ctx, rdb, i)
	}
	elapsed = time.Since(start)
	fmt.Printf("  数据库被访问次数: %d (全部走缓存, 0穿透!)\n", atomic.LoadInt64(&dbHitCount))
	fmt.Printf("  总耗时: %v\n", elapsed)
}
