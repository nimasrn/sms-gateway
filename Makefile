.PHONY: help
help:
	@echo "SMS Gateway - Available Commands"
	@echo "================================="
	@echo "  make run-api          - Run API server"
	@echo "  make run-processor    - Run message processor"
	@echo "  make run-operator     - Run mock operator"
	@echo "  make run-cli          - Run CLI (migrations)"
	@echo ""
	@echo "  make test             - Run all tests"
	@echo "  make test-coverage    - Run tests with coverage"
	@echo ""
	@echo "  make docker-up        - Start all services"
	@echo "  make docker-down      - Stop all services"
	@echo "  make docker-logs      - View service logs"
	@echo ""
	@echo "  make build            - Build all binaries"
	@echo "  make tidy             - Tidy dependencies"

APP_NAME := sms-gateway
BIN_DIR  := bin
CMD_DIR  := cmd

# Run services
.PHONY: run-api
run-api:
	go run ./$(CMD_DIR)/api --env=.env

.PHONY: run-processor
run-processor:
	go run ./$(CMD_DIR)/processor --env=.env

.PHONY: run-operator
run-operator:
	PORT=8081 go run ./$(CMD_DIR)/operator

.PHONY: run-cli
run-cli:
	go run ./$(CMD_DIR)/cli --env=.env --dir=./migrations

# Testing
.PHONY: test
test:
	go test -v ./...

.PHONY: test-coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Docker
.PHONY: docker-up
docker-up:
	docker-compose up -d

.PHONY: docker-down
docker-down:
	docker-compose down

.PHONY: docker-logs
docker-logs:
	docker-compose logs -f

# Build
.PHONY: build
build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -o $(BIN_DIR)/api ./$(CMD_DIR)/api
	CGO_ENABLED=0 go build -o $(BIN_DIR)/processor ./$(CMD_DIR)/processor
	CGO_ENABLED=0 go build -o $(BIN_DIR)/operator ./$(CMD_DIR)/operator
	CGO_ENABLED=0 go build -o $(BIN_DIR)/cli ./$(CMD_DIR)/cli
	@echo "Built: api, processor, operator, cli"

# Utilities
.PHONY: tidy
tidy:
	go mod tidy

.DEFAULT_GOAL := help
