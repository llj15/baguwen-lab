REDIS_LAB := labs/redis-cache-failure

.PHONY: redis-cache-failure redis-cache-failure-verify

redis-cache-failure:
	$(MAKE) -C $(REDIS_LAB) run RESULTS_DIR=./tmp-results

redis-cache-failure-verify:
	$(MAKE) -C $(REDIS_LAB) verify RESULTS_DIR=./tmp-results
