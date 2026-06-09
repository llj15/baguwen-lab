package main

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// ========== 结果结构 ==========

type ScenarioResult struct {
	Name         string  `json:"name"`
	DBHits       int64   `json:"db_hits"`
	DurationMs   float64 `json:"duration_ms"`
	RequestCount int     `json:"request_count"`
	Extra        string  `json:"extra,omitempty"` // 额外指标
}

type ExperimentResult struct {
	Name      string           `json:"name"`
	Scenarios []ScenarioResult `json:"scenarios"`
}

type AllResults struct {
	Timestamp   string             `json:"timestamp"`
	Experiments []ExperimentResult `json:"experiments"`
}

// ========== 全局计数器 ==========

var dbHitCount int64

func resetDBHits() { atomic.StoreInt64(&dbHitCount, 0) }
func getDBHits() int64 { return atomic.LoadInt64(&dbHitCount) }

// ========== 模拟数据库 ==========

var fakeUserDB = map[int]string{
	1: "Alice", 2: "Bob", 3: "Charlie", 4: "David", 5: "Eve",
}

func queryUserDB(id int) (string, bool) {
	atomic.AddInt64(&dbHitCount, 1)
	time.Sleep(5 * time.Millisecond)
	name, ok := fakeUserDB[id]
	return name, ok
}

func queryHotData() string {
	atomic.AddInt64(&dbHitCount, 1)
	time.Sleep(50 * time.Millisecond)
	return "微博热搜: #Redis面试题#"
}

func queryProduct(id int) string {
	atomic.AddInt64(&dbHitCount, 1)
	time.Sleep(10 * time.Millisecond)
	return fmt.Sprintf("商品_%d_信息", id)
}

// ====================================================================
//  布隆过滤器 (Bloom Filter) — 用于穿透防御
//  原理: 多个哈希函数映射到位数组, 不存在的一定不存在, 存在的可能误判
// ====================================================================

type BloomFilter struct {
	bits    []bool
	size    uint
	hashNum uint
}

func NewBloomFilter(size, hashNum uint) *BloomFilter {
	return &BloomFilter{bits: make([]bool, size), size: size, hashNum: hashNum}
}

func (bf *BloomFilter) hash(data string, seed uint) uint {
	h := seed
	for _, c := range data {
		h = h*131 + uint(c) // BKDR hash with different seeds
	}
	return h % bf.size
}

func (bf *BloomFilter) Add(data string) {
	for i := uint(0); i < bf.hashNum; i++ {
		pos := bf.hash(data, i*37+1)
		bf.bits[pos] = true
	}
}

func (bf *BloomFilter) MayExist(data string) bool {
	for i := uint(0); i < bf.hashNum; i++ {
		pos := bf.hash(data, i*37+1)
		if !bf.bits[pos] {
			return false
		}
	}
	return true
}

// ====================================================================
//  本地缓存 (Local Cache) — 用于雪崩防御的 L1 层
//  进程内缓存, 即使 Redis 全挂也有兜底
// ====================================================================

type cacheEntry struct {
	value    string
	expireAt time.Time
}

type LocalCache struct {
	data sync.Map
}

func (lc *LocalCache) Get(key string) (string, bool) {
	v, ok := lc.data.Load(key)
	if !ok {
		return "", false
	}
	entry := v.(*cacheEntry)
	if time.Now().After(entry.expireAt) {
		lc.data.Delete(key)
		return "", false
	}
	return entry.value, true
}

func (lc *LocalCache) Set(key, value string, ttl time.Duration) {
	lc.data.Store(key, &cacheEntry{value: value, expireAt: time.Now().Add(ttl)})
}

// ====================================================================
//  安全锁: owner 标识 + Lua 原子释放 + 看门狗续期
// ====================================================================

func genOwnerID() string {
	b := make([]byte, 8)
	crand.Read(b)
	return fmt.Sprintf("%x", b)
}

// TODO(human): 实现安全释放分布式锁的 Lua 脚本
// 要求: 只有锁的持有者才能释放锁, 防止误删别人的锁
// KEYS[1] = lockKey, ARGV[1] = ownerID
// 逻辑: 如果 GET(lockKey) == ownerID, 则 DEL(lockKey) 并返回 1; 否则返回 0
// 提示: 用 redis.call("GET", KEYS[1]) 获取当前值, 与 ARGV[1] 比较
var releaseLockScript = redis.NewScript(`
	-- 在这里写 Lua 脚本 (3~5行)
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	else
		return 0
	end
`)

// 看门狗: 后台自动续期, 防止业务没做完锁就过期
func startWatchdog(ctx context.Context, rdb *redis.Client, lockKey, ownerID string, ttl time.Duration, done <-chan struct{}) {
	renewInterval := ttl / 3 // 每 TTL/3 续期一次
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// 只有自己持有的锁才续期 (用 Lua 保证原子性)
			renewScript := redis.NewScript(`
				if redis.call("GET", KEYS[1]) == ARGV[1] then
					return redis.call("PEXPIRE", KEYS[1], ARGV[2])
				else
					return 0
				end
			`)
			renewScript.Run(ctx, rdb, []string{lockKey}, ownerID, ttl.Milliseconds())
		}
	}
}

// ====================================================================
//  实验1: 缓存穿透 (Cache Penetration)
//  新增: 布隆过滤器方案
// ====================================================================

func getUserNoProtection(ctx context.Context, rdb *redis.Client, id int) string {
	key := fmt.Sprintf("user:%d", id)
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}
	name, ok := queryUserDB(id)
	if ok {
		rdb.Set(ctx, key, name, 5*time.Minute)
		return name
	}
	return ""
}

const emptyPlaceholder = "<nil>"

func getUserWithEmptyCache(ctx context.Context, rdb *redis.Client, id int) string {
	key := fmt.Sprintf("user:%d", id)
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		if val == emptyPlaceholder {
			return ""
		}
		return val
	}
	name, ok := queryUserDB(id)
	if ok {
		rdb.Set(ctx, key, name, 5*time.Minute)
		return name
	}
	rdb.Set(ctx, key, emptyPlaceholder, 1*time.Minute)
	return ""
}

// 布隆过滤器 + 缓存空值: 双重防护
var bloomRejectCount int64 // 被布隆过滤器拦截的请求数

func getUserWithBloom(ctx context.Context, rdb *redis.Client, bf *BloomFilter, id int) string {
	key := fmt.Sprintf("user:%d", id)

	// 第一层: 布隆过滤器前置拦截
	if !bf.MayExist(key) {
		atomic.AddInt64(&bloomRejectCount, 1)
		return "" // 一定不存在, 直接返回, 连 Redis 都不查
	}

	// 第二层: 查 Redis 缓存
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		if val == emptyPlaceholder {
			return ""
		}
		return val
	}

	// 第三层: 查数据库
	name, ok := queryUserDB(id)
	if ok {
		rdb.Set(ctx, key, name, 5*time.Minute)
		return name
	}
	rdb.Set(ctx, key, emptyPlaceholder, 1*time.Minute)
	return ""
}

func runPenetration(ctx context.Context, rdb *redis.Client) ExperimentResult {
	fmt.Println("\n===== 实验1: 缓存穿透 (Cache Penetration) =====")
	result := ExperimentResult{Name: "缓存穿透"}
	reqCount := 1000

	// --- 场景1: 无保护 ---
	rdb.FlushAll(ctx)
	resetDBHits()
	start := time.Now()
	for i := -reqCount; i < 0; i++ {
		getUserNoProtection(ctx, rdb, i)
	}
	d := time.Since(start)
	h := getDBHits()
	fmt.Printf("  无保护:          DB访问=%d  耗时=%v\n", h, d)
	result.Scenarios = append(result.Scenarios, ScenarioResult{
		Name: "无保护", DBHits: h, DurationMs: float64(d.Milliseconds()), RequestCount: reqCount,
	})

	// --- 场景2: 缓存空值(首轮) ---
	rdb.FlushAll(ctx)
	resetDBHits()
	start = time.Now()
	for i := -reqCount; i < 0; i++ {
		getUserWithEmptyCache(ctx, rdb, i)
	}
	d = time.Since(start)
	h = getDBHits()
	fmt.Printf("  缓存空值(首轮):  DB访问=%d  耗时=%v\n", h, d)
	result.Scenarios = append(result.Scenarios, ScenarioResult{
		Name: "缓存空值_首轮", DBHits: h, DurationMs: float64(d.Milliseconds()), RequestCount: reqCount,
	})

	// --- 场景3: 缓存空值(二轮) ---
	resetDBHits()
	start = time.Now()
	for i := -reqCount; i < 0; i++ {
		getUserWithEmptyCache(ctx, rdb, i)
	}
	d = time.Since(start)
	h = getDBHits()
	fmt.Printf("  缓存空值(二轮):  DB访问=%d  耗时=%v\n", h, d)
	result.Scenarios = append(result.Scenarios, ScenarioResult{
		Name: "缓存空值_二轮", DBHits: h, DurationMs: float64(d.Milliseconds()), RequestCount: reqCount,
	})

	// --- 场景4: 布隆过滤器 + 缓存空值 ---
	rdb.FlushAll(ctx)
	bf := NewBloomFilter(10000, 3) // 10000位, 3个哈希函数
	// 预加载合法 ID 到布隆过滤器
	for id := range fakeUserDB {
		bf.Add(fmt.Sprintf("user:%d", id))
	}

	resetDBHits()
	atomic.StoreInt64(&bloomRejectCount, 0)
	start = time.Now()
	for i := -reqCount; i < 0; i++ {
		getUserWithBloom(ctx, rdb, bf, i)
	}
	d = time.Since(start)
	h = getDBHits()
	rejected := atomic.LoadInt64(&bloomRejectCount)
	fmt.Printf("  布隆过滤器:      DB访问=%d  布隆拦截=%d  耗时=%v\n", h, rejected, d)
	result.Scenarios = append(result.Scenarios, ScenarioResult{
		Name: "布隆过滤器", DBHits: h, DurationMs: float64(d.Milliseconds()), RequestCount: reqCount,
		Extra: fmt.Sprintf("bloom_rejected=%d", rejected),
	})

	return result
}

// ====================================================================
//  实验2: 缓存击穿 (Cache Breakdown)
//  新增: 安全分布式锁 (owner + Lua释放 + 看门狗)
// ====================================================================

var sfGroup singleflight.Group

func getHotNoProtection(ctx context.Context, rdb *redis.Client) string {
	key := "hot:trending"
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}
	data := queryHotData()
	rdb.Set(ctx, key, data, 10*time.Second)
	return data
}

func getHotWithSingleflight(ctx context.Context, rdb *redis.Client) string {
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

// 朴素分布式锁: 有安全隐患 (无owner标识, 可能误删别人的锁)
func getHotWithNaiveLock(ctx context.Context, rdb *redis.Client) string {
	key := "hot:trending"
	lockKey := "lock:" + key

	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}

	ok, _ := rdb.SetNX(ctx, lockKey, "1", 5*time.Second).Result()
	if ok {
		defer rdb.Del(ctx, lockKey) // BUG: 可能删除别人的锁!
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

// 安全分布式锁: owner标识 + Lua原子释放 + 看门狗续期
func getHotWithSafeLock(ctx context.Context, rdb *redis.Client) string {
	key := "hot:trending"
	lockKey := "lock:" + key
	lockTTL := 5 * time.Second

	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}

	// 用唯一 ownerID 标识锁的持有者
	ownerID := genOwnerID()
	ok, _ := rdb.SetNX(ctx, lockKey, ownerID, lockTTL).Result()
	if ok {
		// 启动看门狗, 后台自动续期
		done := make(chan struct{})
		go startWatchdog(ctx, rdb, lockKey, ownerID, lockTTL, done)

		data := queryHotData()
		rdb.Set(ctx, key, data, 10*time.Second)

		close(done) // 停止看门狗
		// 用 Lua 脚本安全释放: 只释放自己的锁
		releaseLockScript.Run(ctx, rdb, []string{lockKey}, ownerID)
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

func runBreakdown(ctx context.Context, rdb *redis.Client) ExperimentResult {
	fmt.Println("\n===== 实验2: 缓存击穿 (Cache Breakdown) =====")
	result := ExperimentResult{Name: "缓存击穿"}
	concurrency := 200

	type testCase struct {
		name string
		fn   func(context.Context, *redis.Client) string
	}

	cases := []testCase{
		{"无保护", getHotNoProtection},
		{"singleflight", getHotWithSingleflight},
		{"朴素分布式锁", getHotWithNaiveLock},
		{"安全分布式锁", getHotWithSafeLock},
	}

	for _, tc := range cases {
		rdb.FlushAll(ctx)
		resetDBHits()
		start := time.Now()

		var wg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tc.fn(ctx, rdb)
			}()
		}
		wg.Wait()

		d := time.Since(start)
		h := getDBHits()
		fmt.Printf("  %-16s: DB访问=%d  耗时=%v  (并发=%d)\n", tc.name, h, d, concurrency)
		result.Scenarios = append(result.Scenarios, ScenarioResult{
			Name: tc.name, DBHits: h, DurationMs: float64(d.Milliseconds()), RequestCount: concurrency,
		})
	}

	return result
}

// ====================================================================
//  实验3: 缓存雪崩 (Cache Avalanche)
//  新增: 多级缓存 (L1本地缓存 + L2 Redis)
// ====================================================================

func warmUpSameTTL(ctx context.Context, rdb *redis.Client, count int) {
	for i := 1; i <= count; i++ {
		key := fmt.Sprintf("product:%d", i)
		rdb.Set(ctx, key, fmt.Sprintf("商品_%d_信息", i), 3*time.Second)
	}
}

func warmUpRandomTTL(ctx context.Context, rdb *redis.Client, count int) {
	for i := 1; i <= count; i++ {
		key := fmt.Sprintf("product:%d", i)
		baseTTL := 3 * time.Second
		jitter := time.Duration(rand.Intn(2000)) * time.Millisecond
		ttl := baseTTL + jitter
		rdb.Set(ctx, key, fmt.Sprintf("商品_%d_信息", i), ttl)
	}
}

func getProduct(ctx context.Context, rdb *redis.Client, id int) string {
	key := fmt.Sprintf("product:%d", id)
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}
	data := queryProduct(id)
	rdb.Set(ctx, key, data, 3*time.Second)
	return data
}

// L1 命中计数
var l1HitCount int64

// 多级缓存查询: L1(本地) → L2(Redis) → DB
func getProductMultiLevel(ctx context.Context, rdb *redis.Client, lc *LocalCache, id int) string {
	key := fmt.Sprintf("product:%d", id)

	// L1: 查本地缓存 (进程内, 纳秒级)
	if val, ok := lc.Get(key); ok {
		atomic.AddInt64(&l1HitCount, 1)
		return val
	}

	// L2: 查 Redis
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		lc.Set(key, val, 5*time.Second) // 回填 L1
		return val
	}

	// DB
	data := queryProduct(id)
	rdb.Set(ctx, key, data, 3*time.Second) // L2
	lc.Set(key, data, 5*time.Second)       // L1 (兜底层, TTL更长)
	return data
}

func runAvalanche(ctx context.Context, rdb *redis.Client) ExperimentResult {
	fmt.Println("\n===== 实验3: 缓存雪崩 (Cache Avalanche) =====")
	result := ExperimentResult{Name: "缓存雪崩"}
	productCount := 200

	// --- 场景1: 相同TTL (无保护) ---
	rdb.FlushAll(ctx)
	warmUpSameTTL(ctx, rdb, productCount)
	fmt.Printf("  预热完成: 相同TTL, %d个key\n", productCount)
	fmt.Println("  等待缓存过期(4秒)...")
	time.Sleep(4 * time.Second)
	keys, _ := rdb.Keys(ctx, "product:*").Result()
	fmt.Printf("  过期后剩余key: %d\n", len(keys))

	resetDBHits()
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
	d := time.Since(start)
	h := getDBHits()
	fmt.Printf("  相同TTL:    DB访问=%d  耗时=%v\n", h, d)
	result.Scenarios = append(result.Scenarios, ScenarioResult{
		Name: "相同TTL", DBHits: h, DurationMs: float64(d.Milliseconds()), RequestCount: productCount,
	})

	// --- 场景2: 随机TTL ---
	rdb.FlushAll(ctx)
	warmUpRandomTTL(ctx, rdb, productCount)
	fmt.Printf("  预热完成: 随机TTL, %d个key\n", productCount)
	fmt.Println("  等待缓存过期(4秒)...")
	time.Sleep(4 * time.Second)
	keys, _ = rdb.Keys(ctx, "product:*").Result()
	fmt.Printf("  过期后剩余key: %d\n", len(keys))

	resetDBHits()
	start = time.Now()
	var wg2 sync.WaitGroup
	for i := 1; i <= productCount; i++ {
		wg2.Add(1)
		go func(id int) {
			defer wg2.Done()
			getProduct(ctx, rdb, id)
		}(i)
	}
	wg2.Wait()
	d = time.Since(start)
	h = getDBHits()
	fmt.Printf("  随机TTL:    DB访问=%d  耗时=%v\n", h, d)
	result.Scenarios = append(result.Scenarios, ScenarioResult{
		Name: "随机TTL", DBHits: h, DurationMs: float64(d.Milliseconds()), RequestCount: productCount,
	})

	// --- 场景3: 随机TTL + 多级缓存 ---
	// 核心思路: L1(本地缓存) 作为 L2(Redis) 的安全网
	// L1 TTL=5s > 等待时间4s, 确保 Redis 过期后 L1 仍能兜底
	// 生产中 L1 用于: 降级兜底 / 减少 Redis 压力 / Redis 宕机时保底
	rdb.FlushAll(ctx)
	lc := &LocalCache{}
	for i := 1; i <= productCount; i++ {
		key := fmt.Sprintf("product:%d", i)
		val := fmt.Sprintf("商品_%d_信息", i)
		baseTTL := 3 * time.Second
		jitter := time.Duration(rand.Intn(2000)) * time.Millisecond
		rdb.Set(ctx, key, val, baseTTL+jitter) // L2: Redis, 3~5s过期
		lc.Set(key, val, 5*time.Second)        // L1: 本地, 5s过期 (兜底)
	}
	fmt.Printf("  预热完成: 随机TTL+多级缓存, %d个key\n", productCount)
	fmt.Println("  等待缓存过期(4秒)...")
	time.Sleep(4 * time.Second)
	keys, _ = rdb.Keys(ctx, "product:*").Result()
	fmt.Printf("  过期后Redis剩余key: %d (L1本地缓存仍全部存活)\n", len(keys))

	resetDBHits()
	atomic.StoreInt64(&l1HitCount, 0)
	start = time.Now()
	var wg3 sync.WaitGroup
	for i := 1; i <= productCount; i++ {
		wg3.Add(1)
		go func(id int) {
			defer wg3.Done()
			getProductMultiLevel(ctx, rdb, lc, id)
		}(i)
	}
	wg3.Wait()
	d = time.Since(start)
	h = getDBHits()
	l1Hits := atomic.LoadInt64(&l1HitCount)
	fmt.Printf("  多级缓存:   DB访问=%d  L1命中=%d  耗时=%v\n", h, l1Hits, d)
	result.Scenarios = append(result.Scenarios, ScenarioResult{
		Name: "多级缓存", DBHits: h, DurationMs: float64(d.Milliseconds()), RequestCount: productCount,
		Extra: fmt.Sprintf("l1_hits=%d", l1Hits),
	})

	return result
}

// ====================================================================
//  主程序
// ====================================================================

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	for i := 0; i < 30; i++ {
		if err := rdb.Ping(ctx).Err(); err == nil {
			break
		}
		fmt.Println("等待Redis就绪...")
		time.Sleep(1 * time.Second)
	}

	fmt.Println("============================================")
	fmt.Println("  Redis 缓存三大问题 实验报告 v2")
	fmt.Println("  穿透 / 击穿 / 雪崩 (增强版)")
	fmt.Println("============================================")

	allResults := AllResults{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
	}

	allResults.Experiments = append(allResults.Experiments, runPenetration(ctx, rdb))
	allResults.Experiments = append(allResults.Experiments, runBreakdown(ctx, rdb))
	allResults.Experiments = append(allResults.Experiments, runAvalanche(ctx, rdb))

	jsonData, _ := json.MarshalIndent(allResults, "", "  ")
	os.WriteFile("/data/results.json", jsonData, 0644)
	fmt.Println("\n\n结果已保存到 /data/results.json")
	fmt.Println("\n===== 所有实验完成 =====")
}
