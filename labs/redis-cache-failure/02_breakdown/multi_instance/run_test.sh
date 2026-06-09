#!/bin/bash
cd "$(dirname "$0")"

echo "编译中..."
go build -o instance . 2>&1

if [ $? -ne 0 ]; then
    echo "编译失败"
    exit 1
fi

# ========== 测试1: singleflight ==========
echo ""
echo "================================================"
echo "  测试1: singleflight (每个进程独立合并)"
echo "  5个实例 x 20并发 = 100个请求"
echo "================================================"

# 清空缓存
docker exec redis-local redis-cli FLUSHALL > /dev/null 2>&1

# 同时启动5个实例
for i in 1 2 3 4 5; do
    ./instance "$i" singleflight &
done
wait

# 统计总的DB访问次数（通过Redis计数器会更准，但这里每个进程独立打印）
echo ""
echo "  singleflight: 每个实例内部合并为1次, 但5个实例仍然有~5次DB访问"

# ========== 测试2: 分布式锁 ==========
echo ""
echo "================================================"
echo "  测试2: Redis分布式锁 (跨进程互斥)"
echo "  5个实例 x 20并发 = 100个请求"
echo "================================================"

docker exec redis-local redis-cli FLUSHALL > /dev/null 2>&1

for i in 1 2 3 4 5; do
    ./instance "$i" distlock &
done
wait

echo ""
echo "  分布式锁: 5个实例中只有1个抢到锁查DB, 其余等待缓存回填"

# 清理
rm -f instance
