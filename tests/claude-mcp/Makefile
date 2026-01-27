# MCP Tool Test Suite
#
# Usage:
#   make test                    # Run all tests in sequence
#   make test-print PROMPT=010   # Run single test in print mode
#   make test-interactive PROMPT=010  # Run single test interactively

SHELL := /bin/bash
PROMPTS_DIR := prompts
RESULTS_DIR := results

# Find all prompt files sorted by number
PROMPTS := $(shell ls -1 $(PROMPTS_DIR)/*.md 2>/dev/null | sort)

# Timestamp for results
TIMESTAMP := $(shell date +%Y-%m-%d-%H-%M-%S)
RUN_DIR := $(RESULTS_DIR)/$(TIMESTAMP)

.PHONY: test test-print test-interactive list clean

# Run all tests in sequence
test:
	@mkdir -p $(RUN_DIR)
	@echo "Running MCP tool tests..."
	@echo "Results will be saved to $(RUN_DIR)"
	@for prompt in $(PROMPTS); do \
		name=$$(basename $$prompt .md); \
		echo ""; \
		echo "=== Running $$name ==="; \
		claude --print "Read the file $$(pwd)/$$prompt and execute the test scenario described." \
			> $(RUN_DIR)/$$name.out 2>&1 || true; \
		echo "Output saved to $(RUN_DIR)/$$name.out"; \
	done
	@echo ""
	@echo "All tests complete. Results in $(RUN_DIR)"

# Run single test in print mode (non-interactive)
test-print:
ifndef PROMPT
	$(error PROMPT is required. Usage: make test-print PROMPT=010)
endif
	@prompt_file=$$(ls -1 $(PROMPTS_DIR)/$(PROMPT)*.md 2>/dev/null | head -1); \
	if [ -z "$$prompt_file" ]; then \
		echo "No prompt file found matching $(PROMPT)"; \
		exit 1; \
	fi; \
	echo "Running $$prompt_file in print mode..."; \
	claude --print "Read the file $$(pwd)/$$prompt_file and execute the test scenario described."

# Run single test interactively
test-interactive:
ifndef PROMPT
	$(error PROMPT is required. Usage: make test-interactive PROMPT=010)
endif
	@prompt_file=$$(ls -1 $(PROMPTS_DIR)/$(PROMPT)*.md 2>/dev/null | head -1); \
	if [ -z "$$prompt_file" ]; then \
		echo "No prompt file found matching $(PROMPT)"; \
		exit 1; \
	fi; \
	echo "Starting interactive session for $$prompt_file..."; \
	echo "Run: Read the file $$(pwd)/$$prompt_file and execute the test scenario described."; \
	claude

# List available prompts
list:
	@echo "Available test prompts:"
	@for prompt in $(PROMPTS); do \
		echo "  $$(basename $$prompt .md)"; \
	done

# Clean results
clean:
	rm -rf $(RESULTS_DIR)/*
