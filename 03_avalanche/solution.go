package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

// ---------- 防雪崩预热：TTL加随机偏移 ----------

func warmUpRandomTTL(ctx context.Context, rdb *redis.Client, count int) {
	for i := 1; i <= count; i++ {
		key := fmt.Sprintf("product:%d", i)

		// TODO(human): 计算一个随机化的TTL
		// 基础TTL 3秒 + 随机偏移 0~2秒，让每个key的过期时间错开
		// 提示: 用 rand.Intn() 生成随机毫秒数，加到基础TTL上
		baseTTL := 3 * time.Second
		ttl := baseTTL // 替换这行，加上随机偏移

		rdb.Set(ctx, key, fmt.Sprintf("商品_%d_信息", i), ttl)
	}
	fmt.Printf("  预热完成: %d个key, TTL=3秒+随机偏移\n", count)
}
