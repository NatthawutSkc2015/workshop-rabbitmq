.PHONY: help test run-producer run-consumer docker-up docker-down setup-queues clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

test: ## Run all tests
	go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	go tool cover -html=coverage.out

run-producer: ## Run producer
	go run cmd/producer/main.go

run-consumer: ## Run consumer
	go run cmd/consumer/main.go

docker-up: ## Start RabbitMQ with docker-compose
	docker-compose up -d
	@echo "Waiting for RabbitMQ to be ready..."
	@sleep 10
	@echo "RabbitMQ Management UI: http://localhost:15672 (guest/guest)"

docker-down: ## Stop RabbitMQ
	docker-compose down

setup-queues: ## Setup RabbitMQ queues and exchanges
	@echo "Setting up RabbitMQ topology..."
	docker exec rabbitmq-demo rabbitmqadmin declare exchange name=demo_exchange type=topic durable=true
	docker exec rabbitmq-demo rabbitmqadmin declare queue name=demo_queue durable=true
	docker exec rabbitmq-demo rabbitmqadmin declare queue name=demo_queue_retry durable=true arguments='{"x-dead-letter-exchange":"","x-dead-letter-routing-key":"demo_queue","x-message-ttl":5000}'
	docker exec rabbitmq-demo rabbitmqadmin declare queue name=demo_queue_dlq durable=true
	docker exec rabbitmq-demo rabbitmqadmin declare binding source=demo_exchange destination=demo_queue routing_key='demo.routing.key'
	@echo "Setup complete!"

clean: ## Clean build artifacts
	rm -f coverage.out

build: ## Build binaries
	go build -o bin/producer cmd/producer/main.go
	go build -o bin/consumer cmd/consumer/main.go

deps: ## Download dependencies
	go mod download
	go mod tidy

lint: ## Run linter (requires golangci-lint)
	golangci-lint run ./...