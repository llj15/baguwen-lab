COMPOSE ?= docker compose
RESULTS_DIR ?= ./results

.PHONY: run build experiment analysis verify config clean

run:
	mkdir -p "$(RESULTS_DIR)"
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) up --build --abort-on-container-exit --exit-code-from experiment experiment
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) up --build --abort-on-container-exit --exit-code-from analysis analysis
	$(COMPOSE) down --remove-orphans

build:
	$(COMPOSE) build

experiment:
	mkdir -p "$(RESULTS_DIR)"
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) up --build --abort-on-container-exit --exit-code-from experiment experiment

analysis:
	mkdir -p "$(RESULTS_DIR)"
	RESULTS_DIR="$(RESULTS_DIR)" $(COMPOSE) up --build --abort-on-container-exit --exit-code-from analysis analysis

verify:
	python scripts/verify_results.py "$(RESULTS_DIR)/results.json"

config:
	$(COMPOSE) config

clean:
	$(COMPOSE) down --volumes --remove-orphans
