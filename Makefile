REDIS_LAB := labs/redis-cache-failure
REDIS_LOCK_LAB := labs/redis-distributed-lock

.PHONY: redis-cache-failure redis-cache-failure-verify redis-distributed-lock redis-distributed-lock-verify

redis-cache-failure:
	$(MAKE) -C $(REDIS_LAB) run RESULTS_DIR=./tmp-results

redis-cache-failure-verify:
	$(MAKE) -C $(REDIS_LAB) verify RESULTS_DIR=./tmp-results

redis-distributed-lock:
	$(MAKE) -C $(REDIS_LOCK_LAB) run RESULTS_DIR=./tmp-results

redis-distributed-lock-verify:
	$(MAKE) -C $(REDIS_LOCK_LAB) verify RESULTS_DIR=./tmp-results
