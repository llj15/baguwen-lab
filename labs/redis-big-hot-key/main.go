package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

type DatasetConfig struct {
	NormalStringCount int `json:"normal_string_count"`
	NormalValueBytes  int `json:"normal_value_bytes"`
	BigStringBytes    int `json:"big_string_bytes"`
	BigHashFields     int `json:"big_hash_fields"`
	BigHashBuckets    int `json:"big_hash_buckets"`
	BigListItems      int `json:"big_list_items"`
	BigElementBytes   int `json:"big_element_bytes"`
	HotKeyspace       int `json:"hot_keyspace"`
	HotRequests       int `json:"hot_requests"`
	HotRatioPercent   int `json:"hot_ratio_percent"`
	HotShards         int `json:"hot_shards"`
}

type Scenario struct {
	Name    string         `json:"name"`
	Metrics map[string]any `json:"metrics"`
}

type Experiment struct {
	Name      string     `json:"name"`
	Scenarios []Scenario `json:"scenarios"`
}

type ResultFile struct {
	SchemaVersion int           `json:"schema_version"`
	Lab           string        `json:"lab"`
	Seed          int           `json:"seed"`
	GeneratedAt   string        `json:"generated_at"`
	Dataset       DatasetConfig `json:"dataset"`
	Experiments   []Experiment  `json:"experiments"`
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
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

func payload(size int, marker byte) string {
	return strings.Repeat(string([]byte{marker}), size)
}

func execPipeline(rdb *redis.Client, pipe redis.Pipeliner) {
	if _, err := pipe.Exec(ctx); err != nil {
		panic(err)
	}
}

func memoryUsage(rdb *redis.Client, key string) int64 {
	value, err := rdb.Do(ctx, "MEMORY", "USAGE", key).Int64()
	if err != nil {
		panic(fmt.Errorf("MEMORY USAGE %s: %w", key, err))
	}
	return value
}

func durationMs(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

func createNormalStrings(rdb *redis.Client, cfg DatasetConfig) int64 {
	value := payload(cfg.NormalValueBytes, 'n')
	for offset := 0; offset < cfg.NormalStringCount; offset += 1000 {
		pipe := rdb.Pipeline()
		limit := offset + 1000
		if limit > cfg.NormalStringCount {
			limit = cfg.NormalStringCount
		}
		for i := offset; i < limit; i++ {
			pipe.Set(ctx, fmt.Sprintf("normal:string:%05d", i), value, 0)
		}
		execPipeline(rdb, pipe)
	}

	var total int64
	samples := 100
	for i := 0; i < samples; i++ {
		total += memoryUsage(rdb, fmt.Sprintf("normal:string:%05d", i))
	}
	return total / int64(samples)
}

func createHash(rdb *redis.Client, key string, fields int, value string) {
	for offset := 0; offset < fields; offset += 1000 {
		pipe := rdb.Pipeline()
		limit := offset + 1000
		if limit > fields {
			limit = fields
		}
		for i := offset; i < limit; i++ {
			pipe.HSet(ctx, key, fmt.Sprintf("field:%05d", i), value)
		}
		execPipeline(rdb, pipe)
	}
}

func createBucketedHash(rdb *redis.Client, prefix string, fields int, buckets int, value string) {
	for offset := 0; offset < fields; offset += 1000 {
		pipe := rdb.Pipeline()
		limit := offset + 1000
		if limit > fields {
			limit = fields
		}
		for i := offset; i < limit; i++ {
			bucket := i % buckets
			key := fmt.Sprintf("%s:%03d", prefix, bucket)
			pipe.HSet(ctx, key, fmt.Sprintf("field:%05d", i), value)
		}
		execPipeline(rdb, pipe)
	}
}

func createList(rdb *redis.Client, key string, items int, value string) {
	for offset := 0; offset < items; offset += 1000 {
		limit := offset + 1000
		if limit > items {
			limit = items
		}
		values := make([]any, 0, limit-offset)
		for i := offset; i < limit; i++ {
			values = append(values, fmt.Sprintf("%s:%05d", value, i))
		}
		if err := rdb.RPush(ctx, key, values...).Err(); err != nil {
			panic(err)
		}
	}
}

func bucketStats(rdb *redis.Client, prefix string, buckets int) (int64, int64, int64) {
	var minFields int64 = 1<<63 - 1
	var maxFields int64
	var totalFields int64
	for i := 0; i < buckets; i++ {
		fields, err := rdb.HLen(ctx, fmt.Sprintf("%s:%03d", prefix, i)).Result()
		if err != nil {
			panic(err)
		}
		if fields < minFields {
			minFields = fields
		}
		if fields > maxFields {
			maxFields = fields
		}
		totalFields += fields
	}
	return minFields, maxFields, totalFields
}

func scanHash(rdb *redis.Client, key string, count int64) (int, int, int64) {
	start := time.Now()
	cursor := uint64(0)
	seen := 0
	iterations := 0
	for {
		values, next, err := rdb.HScan(ctx, key, cursor, "*", count).Result()
		if err != nil {
			panic(err)
		}
		seen += len(values) / 2
		iterations++
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return seen, iterations, durationMs(start)
}

func runBigKeyExperiment(rdb *redis.Client, cfg DatasetConfig) Experiment {
	fmt.Println("===== Experiment 1: Redis Big Key =====")
	normalStart := time.Now()
	normalAvgMemory := createNormalStrings(rdb, cfg)
	normalWriteMs := durationMs(normalStart)

	bigValue := payload(cfg.BigStringBytes, 's')
	start := time.Now()
	if err := rdb.Set(ctx, "big:string:8m", bigValue, 0).Err(); err != nil {
		panic(err)
	}
	bigStringWriteMs := durationMs(start)
	bigStringMemory := memoryUsage(rdb, "big:string:8m")
	bigStringLen, err := rdb.StrLen(ctx, "big:string:8m").Result()
	if err != nil {
		panic(err)
	}

	hashValue := payload(cfg.BigElementBytes, 'h')
	start = time.Now()
	createHash(rdb, "big:hash:50k", cfg.BigHashFields, hashValue)
	bigHashWriteMs := durationMs(start)
	bigHashMemory := memoryUsage(rdb, "big:hash:50k")
	bigHashFields, err := rdb.HLen(ctx, "big:hash:50k").Result()
	if err != nil {
		panic(err)
	}

	start = time.Now()
	createBucketedHash(rdb, "split:hash:50k", cfg.BigHashFields, cfg.BigHashBuckets, hashValue)
	splitHashWriteMs := durationMs(start)
	minBucketFields, maxBucketFields, splitTotalFields := bucketStats(rdb, "split:hash:50k", cfg.BigHashBuckets)

	listValue := payload(cfg.BigElementBytes, 'l')
	start = time.Now()
	createList(rdb, "big:list:50k", cfg.BigListItems, listValue)
	bigListWriteMs := durationMs(start)
	bigListMemory := memoryUsage(rdb, "big:list:50k")
	bigListItems, err := rdb.LLen(ctx, "big:list:50k").Result()
	if err != nil {
		panic(err)
	}

	start = time.Now()
	allHash, err := rdb.HGetAll(ctx, "big:hash:50k").Result()
	if err != nil {
		panic(err)
	}
	hgetallMs := durationMs(start)
	scanCount, scanIterations, hscanMs := scanHash(rdb, "big:hash:50k", 500)

	fmt.Printf("Normal strings: count=%d avg_memory=%dB write=%dms\n", cfg.NormalStringCount, normalAvgMemory, normalWriteMs)
	fmt.Printf("Big string: len=%d memory=%dB write=%dms\n", bigStringLen, bigStringMemory, bigStringWriteMs)
	fmt.Printf("Big hash: fields=%d memory=%dB write=%dms hgetall=%dms hscan=%dms iterations=%d\n", bigHashFields, bigHashMemory, bigHashWriteMs, hgetallMs, hscanMs, scanIterations)
	fmt.Printf("Big list: items=%d memory=%dB write=%dms\n", bigListItems, bigListMemory, bigListWriteMs)

	return Experiment{
		Name: "redis_big_key",
		Scenarios: []Scenario{
			{
				Name: "representative_dataset",
				Metrics: map[string]any{
					"normal_string_count": cfg.NormalStringCount,
					"normal_value_bytes":  cfg.NormalValueBytes,
					"normal_avg_memory":   normalAvgMemory,
					"normal_write_ms":     normalWriteMs,
				},
			},
			{
				Name: "big_string",
				Metrics: map[string]any{
					"key":              "big:string:8m",
					"logical_bytes":    cfg.BigStringBytes,
					"strlen":           bigStringLen,
					"memory_bytes":     bigStringMemory,
					"memory_vs_normal": float64(bigStringMemory) / float64(normalAvgMemory),
					"write_ms":         bigStringWriteMs,
				},
			},
			{
				Name: "big_hash",
				Metrics: map[string]any{
					"key":              "big:hash:50k",
					"fields":           bigHashFields,
					"element_bytes":    cfg.BigElementBytes,
					"memory_bytes":     bigHashMemory,
					"memory_vs_normal": float64(bigHashMemory) / float64(normalAvgMemory),
					"write_ms":         bigHashWriteMs,
				},
			},
			{
				Name: "big_list",
				Metrics: map[string]any{
					"key":              "big:list:50k",
					"items":            bigListItems,
					"element_bytes":    cfg.BigElementBytes,
					"memory_bytes":     bigListMemory,
					"memory_vs_normal": float64(bigListMemory) / float64(normalAvgMemory),
					"write_ms":         bigListWriteMs,
				},
			},
			{
				Name: "split_hash_mitigation",
				Metrics: map[string]any{
					"bucket_count":      cfg.BigHashBuckets,
					"total_fields":      splitTotalFields,
					"min_bucket_fields": minBucketFields,
					"max_bucket_fields": maxBucketFields,
					"write_ms":          splitHashWriteMs,
				},
			},
			{
				Name: "hash_full_read_vs_scan",
				Metrics: map[string]any{
					"hgetall_fields":   len(allHash),
					"hgetall_ms":       hgetallMs,
					"hscan_fields":     scanCount,
					"hscan_count":      500,
					"hscan_iterations": scanIterations,
					"hscan_ms":         hscanMs,
				},
			},
		},
	}
}

func createHotKeys(rdb *redis.Client, cfg DatasetConfig) {
	value := payload(256, 'p')
	for offset := 0; offset < cfg.HotKeyspace; offset += 1000 {
		pipe := rdb.Pipeline()
		limit := offset + 1000
		if limit > cfg.HotKeyspace {
			limit = cfg.HotKeyspace
		}
		for i := offset; i < limit; i++ {
			pipe.Set(ctx, fmt.Sprintf("item:%04d", i), value, 0)
		}
		execPipeline(rdb, pipe)
	}

	pipe := rdb.Pipeline()
	for i := 0; i < cfg.HotShards; i++ {
		pipe.Set(ctx, fmt.Sprintf("item:hot:%02d", i), value, 0)
	}
	execPipeline(rdb, pipe)
}

func topStats(counts map[string]int, total int) (string, int, float64, int) {
	type pair struct {
		key   string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for key, count := range counts {
		pairs = append(pairs, pair{key: key, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].count > pairs[j].count
	})
	if len(pairs) == 0 {
		return "", 0, 0, 0
	}
	return pairs[0].key, pairs[0].count, float64(pairs[0].count) / float64(total), len(pairs)
}

func executeGets(rdb *redis.Client, keys []string) {
	for offset := 0; offset < len(keys); offset += 1000 {
		pipe := rdb.Pipeline()
		limit := offset + 1000
		if limit > len(keys) {
			limit = len(keys)
		}
		for _, key := range keys[offset:limit] {
			pipe.Get(ctx, key)
		}
		execPipeline(rdb, pipe)
	}
}

func runAccessPattern(rdb *redis.Client, name string, keys []string) Scenario {
	counts := map[string]int{}
	for _, key := range keys {
		counts[key]++
	}
	start := time.Now()
	executeGets(rdb, keys)
	elapsedMs := durationMs(start)
	topKey, topCount, topRatio, uniqueKeys := topStats(counts, len(keys))
	return Scenario{
		Name: name,
		Metrics: map[string]any{
			"requests":    len(keys),
			"unique_keys": uniqueKeys,
			"top_key":     topKey,
			"top_count":   topCount,
			"top_ratio":   topRatio,
			"elapsed_ms":  elapsedMs,
		},
	}
}

func shardedTopStats(counts map[string]int, prefix string, shards int) (int, int) {
	total := 0
	maxShard := 0
	for i := 0; i < shards; i++ {
		key := fmt.Sprintf("%s:%02d", prefix, i)
		count := counts[key]
		total += count
		if count > maxShard {
			maxShard = count
		}
	}
	return total, maxShard
}

func uniformKeys(cfg DatasetConfig) []string {
	rng := rand.New(rand.NewSource(20260609))
	keys := make([]string, 0, cfg.HotRequests)
	for i := 0; i < cfg.HotRequests; i++ {
		keys = append(keys, fmt.Sprintf("item:%04d", rng.Intn(cfg.HotKeyspace)))
	}
	return keys
}

func hotKeys(cfg DatasetConfig) []string {
	rng := rand.New(rand.NewSource(20260610))
	keys := make([]string, 0, cfg.HotRequests)
	for i := 0; i < cfg.HotRequests; i++ {
		if i%100 < cfg.HotRatioPercent {
			keys = append(keys, "item:0000")
			continue
		}
		keys = append(keys, fmt.Sprintf("item:%04d", 1+rng.Intn(cfg.HotKeyspace-1)))
	}
	return keys
}

func shardedHotKeys(cfg DatasetConfig) []string {
	rng := rand.New(rand.NewSource(20260611))
	keys := make([]string, 0, cfg.HotRequests)
	for i := 0; i < cfg.HotRequests; i++ {
		if i%100 < cfg.HotRatioPercent {
			keys = append(keys, fmt.Sprintf("item:hot:%02d", i%cfg.HotShards))
			continue
		}
		keys = append(keys, fmt.Sprintf("item:%04d", 1+rng.Intn(cfg.HotKeyspace-1)))
	}
	return keys
}

func runHotKeyExperiment(rdb *redis.Client, cfg DatasetConfig) Experiment {
	fmt.Println("===== Experiment 2: Redis Hot Key =====")
	start := time.Now()
	createHotKeys(rdb, cfg)
	setupMs := durationMs(start)

	uniform := runAccessPattern(rdb, "uniform_access", uniformKeys(cfg))
	hot := runAccessPattern(rdb, "single_hot_key_access", hotKeys(cfg))
	shardedKeys := shardedHotKeys(cfg)
	sharded := runAccessPattern(rdb, "sharded_hot_key_access", shardedKeys)
	shardedCounts := map[string]int{}
	for _, key := range shardedKeys {
		shardedCounts[key]++
	}
	totalCopyGets, maxCopyGets := shardedTopStats(shardedCounts, "item:hot", cfg.HotShards)
	dataset := Scenario{
		Name: "representative_dataset",
		Metrics: map[string]any{
			"keyspace":          cfg.HotKeyspace,
			"requests":          cfg.HotRequests,
			"hot_ratio_percent": cfg.HotRatioPercent,
			"hot_shards":        cfg.HotShards,
			"setup_ms":          setupMs,
		},
	}

	fmt.Printf("Uniform access: top=%s ratio=%.4f requests=%d\n", uniform.Metrics["top_key"], uniform.Metrics["top_ratio"], uniform.Metrics["requests"])
	fmt.Printf("Hot key access: top=%s ratio=%.4f requests=%d\n", hot.Metrics["top_key"], hot.Metrics["top_ratio"], hot.Metrics["requests"])
	fmt.Printf("Sharded hot key access: top=%s ratio=%.4f shards=%d\n", sharded.Metrics["top_key"], sharded.Metrics["top_ratio"], cfg.HotShards)

	sharded.Metrics["hot_copy_count"] = cfg.HotShards
	sharded.Metrics["hot_copy_gets"] = totalCopyGets
	sharded.Metrics["max_copy_gets"] = maxCopyGets

	return Experiment{
		Name:      "redis_hot_key",
		Scenarios: []Scenario{dataset, uniform, hot, sharded},
	}
}

func main() {
	cfg := DatasetConfig{
		NormalStringCount: 20000,
		NormalValueBytes:  128,
		BigStringBytes:    8 * 1024 * 1024,
		BigHashFields:     50000,
		BigHashBuckets:    100,
		BigListItems:      50000,
		BigElementBytes:   64,
		HotKeyspace:       10000,
		HotRequests:       100000,
		HotRatioPercent:   60,
		HotShards:         16,
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		panic(err)
	}

	fmt.Println("==========================================")
	fmt.Println("  Redis Big Key and Hot Key Lab")
	fmt.Println("==========================================")
	fmt.Printf("Redis: %s\n", redisAddr())

	results := ResultFile{
		SchemaVersion: 1,
		Lab:           "redis-big-hot-key",
		Seed:          20260609,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Dataset:       cfg,
		Experiments: []Experiment{
			runBigKeyExperiment(rdb, cfg),
			runHotKeyExperiment(rdb, cfg),
		},
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		panic(err)
	}
	path := outputPath("results.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Results written to %s\n", path)
}
