package rabbitmq

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	RabbitMQURL      string
	Exchange         string
	Queue            string
	RoutingKey       string
	RetryQueue       string
	DLQ              string
	PrefetchCount    int
	WorkerCount      int
	MaxRetries       int
	RetryDelay       time.Duration
	ReconnectBackoff time.Duration
	MaxReconnectWait time.Duration
}

func Load() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found")
		os.Exit(1)
	}

	return &Config{
		RabbitMQURL:      getEnv("RABBITMQ_URL", "amqp://admin:password123@localhost:5672/"),
		Exchange:         getEnv("EXCHANGE", "demo_exchange"),
		Queue:            getEnv("QUEUE", "demo_queue"),
		RoutingKey:       getEnv("ROUTING_KEY", "demo.routing.key"),
		RetryQueue:       getEnv("RETRY_QUEUE", "demo_queue_retry"),
		DLQ:              getEnv("DLQ", "demo_queue_dlq"),
		PrefetchCount:    getEnvInt("PREFETCH_COUNT", 10),
		WorkerCount:      getEnvInt("WORKER_COUNT", 1),
		MaxRetries:       getEnvInt("MAX_RETRIES", 3),
		RetryDelay:       time.Duration(getEnvInt("RETRY_DELAY_MS", 1000)) * time.Millisecond,
		ReconnectBackoff: time.Duration(getEnvInt("RECONNECT_BACKOFF_MS", 1000)) * time.Millisecond,
		MaxReconnectWait: time.Duration(getEnvInt("MAX_RECONNECT_WAIT_MS", 30000)) * time.Millisecond,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
