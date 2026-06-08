package main

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

var sfGroup singleflight.Group

func getHotDataWithProtection(ctx context.Context, rdb *redis.Client) string {
	key := "hot:trending"

	// 1. 查缓存
	val, err := rdb.Get(ctx, key).Result()
	if err == nil {
		return val
	}

	// 2. 缓存miss → 用 singleflight 保证只有一个请求查DB
	// TODO(human): 使用 sfGroup.Do() 包裹数据库查询逻辑
	// sfGroup.Do(key, func() (interface{}, error) { ... })
	// 在回调里: 查询数据库 → 写入缓存 → 返回结果
	// Do() 会保证同一个 key 的并发调用只执行一次回调，其余等待共享结果
	result, _, _ := sfGroup.Do(key, func() (interface{}, error) {
		data := queryHotData(ctx)
		rdb.Set(ctx, key, data, 3*time.Second)
		return data, nil
	})

	return result.(string)
}
