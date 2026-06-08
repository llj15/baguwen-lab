package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const emptyPlaceholder = "<nil>" // 用一个特殊标记代表"数据库里没有"

func getUserWithProtection(ctx context.Context, rdb *redis.Client, id int) string {
	key := fmt.Sprintf("user:%d", id)

	// 1. 先查缓存
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		if val == emptyPlaceholder {
			return "" // 命中空值缓存，直接返回，不穿透
		}
		return val
	}

	// 2. 缓存未命中，查数据库
	name, ok := queryDB(id)
	if ok {
		rdb.Set(ctx, key, name, 5*time.Minute)
		return name
	}

	// TODO(human): 数据库也没查到，在这里实现防穿透逻辑
	// 提示: 把 emptyPlaceholder 写入 Redis，设置一个较短的过期时间
	// 这样下次同样的请求就能命中缓存，不会再打到数据库
	rdb.Set(ctx, key, emptyPlaceholder, 1*time.Minute)

	return ""
}
