#!/bin/bash

# Setup RabbitMQ Topology for Demo
# This script creates exchanges, queues, and bindings

set -e

RABBITMQ_HOST=${RABBITMQ_HOST:-localhost}
RABBITMQ_PORT=${RABBITMQ_PORT:-15672}
RABBITMQ_USER=${RABBITMQ_USER:-admin}
RABBITMQ_PASS=${RABBITMQ_PASS:-password123}

EXCHANGE=${EXCHANGE:-demo_exchange}
QUEUE=${QUEUE:-demo_queue}
RETRY_QUEUE=${RETRY_QUEUE:-demo_queue_retry}
DLQ=${DLQ:-demo_queue_dlq}
ROUTING_KEY=${ROUTING_KEY:-demo.routing.key}

echo "Setting up RabbitMQ topology..."
echo "Host: $RABBITMQ_HOST:$RABBITMQ_PORT"

# Function to make API calls
rabbitmq_api() {
    curl -s -u "$RABBITMQ_USER:$RABBITMQ_PASS" \
        -H "content-type:application/json" \
        "$@"
}

# Create exchange
echo "Creating exchange: $EXCHANGE"
rabbitmq_api -X PUT \
    "http://$RABBITMQ_HOST:$RABBITMQ_PORT/api/exchanges/%2F/$EXCHANGE" \
    -d '{
        "type": "topic",
        "durable": true,
        "auto_delete": false
    }'

# Create main queue
echo "Creating main queue: $QUEUE"
rabbitmq_api -X PUT \
    "http://$RABBITMQ_HOST:$RABBITMQ_PORT/api/queues/%2F/$QUEUE" \
    -d '{
        "durable": true,
        "auto_delete": false
    }'

# Create retry queue with DLX and TTL
echo "Creating retry queue: $RETRY_QUEUE"
rabbitmq_api -X PUT \
    "http://$RABBITMQ_HOST:$RABBITMQ_PORT/api/queues/%2F/$RETRY_QUEUE" \
    -d "{
        \"durable\": true,
        \"auto_delete\": false,
        \"arguments\": {
            \"x-dead-letter-exchange\": \"\",
            \"x-dead-letter-routing-key\": \"$QUEUE\",
            \"x-message-ttl\": 5000
        }
    }"

# Create DLQ
echo "Creating dead letter queue: $DLQ"
rabbitmq_api -X PUT \
    "http://$RABBITMQ_HOST:$RABBITMQ_PORT/api/queues/%2F/$DLQ" \
    -d '{
        "durable": true,
        "auto_delete": false
    }'

# Bind main queue to exchange
echo "Binding $QUEUE to $EXCHANGE with routing key: $ROUTING_KEY"
rabbitmq_api -X POST \
    "http://$RABBITMQ_HOST:$RABBITMQ_PORT/api/bindings/%2F/e/$EXCHANGE/q/$QUEUE" \
    -d "{
        \"routing_key\": \"$ROUTING_KEY\"
    }"

echo ""
echo "✅ RabbitMQ topology setup complete!"
echo ""
echo "Summary:"
echo "  Exchange: $EXCHANGE (topic)"
echo "  Main Queue: $QUEUE"
echo "  Retry Queue: $RETRY_QUEUE (TTL: 5s)"
echo "  Dead Letter Queue: $DLQ"
echo "  Binding: $EXCHANGE → $QUEUE ($ROUTING_KEY)"
echo ""
echo "Access RabbitMQ Management UI: http://$RABBITMQ_HOST:$RABBITMQ_PORT"