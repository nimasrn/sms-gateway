.PHONY: test build clean stop logs help

# Configuration - Edit these values
URL ?= http://host.docker.internal:8080/api/v1/messages/express
RPS ?= 10
DURATION ?= 80000
WORKERS ?= 10
API_KEY ?=

help: ## Show this help
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  make test       - Run load test (5000 RPS, 30 sec)"
	@echo "  make test-10k   - Run high load test (10k RPS)"
	@echo "  make test-long  - Run long test (5 min)"
	@echo "  make logs       - View test logs"
	@echo "  make stop       - Stop the test"
	@echo "  make clean      - Clean up everything"
	@echo ""
	@echo "Custom settings:"
	@echo "  make test RPS=10000 DURATION=60 WORKERS=1000"

test: ## Run load test
	@echo "🚀 Starting load test..."
	@echo "  URL: $(URL)"
	@echo "  RPS: $(RPS)"
	@echo "  Duration: $(DURATION)s"
	@echo "  Workers: $(WORKERS)"
	@echo ""
	@TARGET_URL=$(URL) REQUESTS_PER_SECOND=$(RPS) DURATION_SECONDS=$(DURATION) CONCURRENT_WORKERS=$(WORKERS) API_KEY=$(API_KEY) docker-compose -f docker-compose.yml up --build

test-10k: ## Run 10k RPS test
	@make test RPS=10000 WORKERS=1000

test-long: ## Run 5 minute test
	@make test RPS=1000 DURATION=300

build: ## Build the Docker image
	@docker-compose -f docker-compose.simple.yml build

logs: ## View logs
	@docker-compose -f docker-compose.simple.yml logs -f

stop: ## Stop the test
	@docker-compose -f docker-compose.simple.yml down

clean: ## Clean up everything
	@docker-compose -f docker-compose.simple.yml down -v
	@docker system prune -f
	@echo "✅ Cleaned up!"
