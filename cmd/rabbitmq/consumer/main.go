package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	config "go-queue/configs/rabbitmq"

	"go-queue/pkg/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

type Message struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	// Setup logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)

	// Load config
	cfg := config.Load()
	logger.WithField("url", cfg.RabbitMQURL).Info("Starting consumer")

	// Create connector
	factory := &rabbitmq.RealConnectionFactory{}
	connector := rabbitmq.NewConnector(
		cfg.RabbitMQURL,
		factory,
		logger,
		cfg.ReconnectBackoff,
		cfg.MaxReconnectWait,
	)

	if err := connector.Connect(); err != nil {
		logger.WithError(err).Fatal("Failed to connect to RabbitMQ")
	}
	defer connector.Close()

	// Create message handler
	handler := func(ctx context.Context, delivery amqp.Delivery) error {
		var msg Message
		if err := json.Unmarshal(delivery.Body, &msg); err != nil {
			logger.WithError(err).Error("Failed to unmarshal message")
			return err
		}

		logger.WithFields(logrus.Fields{
			"message_id": msg.ID,
			"content":    msg.Content,
			"timestamp":  msg.Timestamp,
		}).Info("Processing message")

		// Simulate processing time
		time.Sleep(100 * time.Millisecond)

		// Simulate random failures (10% failure rate for demo)
		// Comment out in production
		// if rand.Intn(10) == 0 {
		// 	return errors.New("simulated processing error")
		// }

		logger.WithField("message_id", msg.ID).Info("Message processed successfully")
		return nil
	}

	// Create consumer
	metrics := rabbitmq.NewSimpleMetrics()
	dedupeStore := rabbitmq.NewInMemoryDedupeStore()

	consumer := rabbitmq.NewConsumer(
		connector,
		cfg.Queue,
		cfg.RetryQueue,
		cfg.DLQ,
		cfg.PrefetchCount,
		cfg.WorkerCount,
		cfg.MaxRetries,
		handler,
		dedupeStore,
		logger,
		metrics,
	)

	// Start consumer
	if err := consumer.Start(); err != nil {
		logger.WithError(err).Fatal("Failed to start consumer")
	}

	logger.WithFields(logrus.Fields{
		"queue":          cfg.Queue,
		"workers":        cfg.WorkerCount,
		"prefetch_count": cfg.PrefetchCount,
	}).Info("Consumer started successfully")

	// Print metrics periodically
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			printMetrics(logger, metrics)
		}
	}()

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logger.Info("Shutting down consumer...")

	// Stop consumer
	consumer.Stop()

	printMetrics(logger, metrics)
	logger.Info("Consumer stopped")
}

func printMetrics(logger *logrus.Logger, metrics *rabbitmq.SimpleMetrics) {
	logger.WithFields(logrus.Fields{
		"received":  metrics.GetReceived(),
		"processed": metrics.GetProcessed(),
		"failed":    metrics.GetFailed(),
		"retried":   metrics.GetRetried(),
	}).Info("Consumer metrics")
}
