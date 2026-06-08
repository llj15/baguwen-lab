package main

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

func getHotDataWithDistLock(ctx context.Context, rdb *redis.Client) string {
	key := "hot:trending"
	lockKey := "lock:" + key

	// 1. 查缓存
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}

	// 2. 缓存miss → 尝试抢分布式锁
	// SET lockKey 1 NX EX 5  → 只有一个请求能抢到
	ok, _ := rdb.SetNX(ctx, lockKey, "1", 5*time.Second).Result()

	if ok {
		// 抢到锁 → 我来查DB、回填缓存
		defer rdb.Del(ctx, lockKey) // 查完释放锁

		data := queryHotData(ctx)
		rdb.Set(ctx, key, data, 3*time.Second)
		return data
	}

	// 没抢到锁 → 等一会儿重试读缓存（等持锁者回填完）
	// TODO(human): 实现等待重试逻辑
	// 提示: 循环等待，每次 sleep 一小段时间后重新查缓存
	// 如果查到了就返回，超过一定次数就降级直接查DB
	for i := 0; i < 20; i++ {
		time.Sleep(10 * time.Millisecond)
		val, err = rdb.Get(ctx, key).Result()
		if err == nil {
			return val
		}
	}

	// 兜底: 等太久了，降级直接查DB
	return queryHotData(ctx)
}
