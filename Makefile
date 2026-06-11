REDIS_LAB := labs/redis-cache-failure
REDIS_LOCK_LAB := labs/redis-distributed-lock
REDIS_BIG_HOT_KEY_LAB := labs/redis-big-hot-key
KAFKA_DEMO_LAB := labs/kafka-demo

.PHONY: redis-cache-failure redis-cache-failure-verify redis-distributed-lock redis-distributed-lock-verify redis-big-hot-key redis-big-hot-key-verify kafka-demo kafka-demo-verify

redis-cache-failure:
	$(MAKE) -C $(REDIS_LAB) run RESULTS_DIR=./tmp-results

redis-cache-failure-verify:
	$(MAKE) -C $(REDIS_LAB) verify RESULTS_DIR=./tmp-results

redis-distributed-lock:
	$(MAKE) -C $(REDIS_LOCK_LAB) run RESULTS_DIR=./tmp-results

redis-distributed-lock-verify:
	$(MAKE) -C $(REDIS_LOCK_LAB) verify RESULTS_DIR=./tmp-results

redis-big-hot-key:
	$(MAKE) -C $(REDIS_BIG_HOT_KEY_LAB) run RESULTS_DIR=./tmp-results

redis-big-hot-key-verify:
	$(MAKE) -C $(REDIS_BIG_HOT_KEY_LAB) verify RESULTS_DIR=./tmp-results

kafka-demo:
	$(MAKE) -C $(KAFKA_DEMO_LAB) run RESULTS_DIR=./tmp-results

kafka-demo-verify:
	$(MAKE) -C $(KAFKA_DEMO_LAB) verify RESULTS_DIR=./tmp-results
