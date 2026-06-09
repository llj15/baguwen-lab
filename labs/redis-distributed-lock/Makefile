COMPOSE ?= docker compose
RESULTS_DIR ?= ./results

.PHONY: run build experiment analysis verify config clean

run:
	mkdir -p "$(RESULTS_DIR)"
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) up --build --abort-on-container-exit --exit-code-from experiment experiment
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) up --build --no-deps --abort-on-container-exit --exit-code-from analysis analysis
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) run --rm --no-deps analysis python /app/scripts/verify_results.py /data --baseline /baseline
	$(COMPOSE) down --remove-orphans

build:
	$(COMPOSE) build

experiment:
	mkdir -p "$(RESULTS_DIR)"
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) up --build --abort-on-container-exit --exit-code-from experiment experiment

analysis:
	mkdir -p "$(RESULTS_DIR)"
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) up --build --no-deps --abort-on-container-exit --exit-code-from analysis analysis

verify:
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) run --rm --no-deps analysis python /app/scripts/verify_results.py /data --baseline /baseline

config:
	$(COMPOSE) config

clean:
	$(COMPOSE) down --volumes --remove-orphans
