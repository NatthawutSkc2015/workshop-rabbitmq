package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	config "go-queue/configs/rabbitmq"
	"go-queue/internal/service"
	"go-queue/pkg/rabbitmq"

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
	logger.WithField("url", cfg.RabbitMQURL).Info("Starting producer")

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

	// Create producer
	metrics := rabbitmq.NewSimpleMetrics()
	producer := rabbitmq.NewProducer(
		connector,
		logger,
		metrics,
		cfg.MaxRetries,
		cfg.RetryDelay,
	)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Publish messages
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-sigChan
		logger.Info("Shutting down producer...")
		cancel()
	}()

	// Usecase service
	id := time.Second
	msg := Message{
		ID:      fmt.Sprintf("msg-%d", id),
		Content: fmt.Sprintf("Hello RabbitMQ! Message #%d", id),
	}
	newMessageService := service.NewMessageService(producer, cfg.Exchange, cfg.RoutingKey)
	if err := newMessageService.SendMessage(ctx, msg.ID, msg.Content); err != nil {
		logger.WithError(err).Error("Failed to publish message")
	}
	logger.WithField("message_id", msg.ID).Info("Message published successfully")

}

func printMetrics(logger *logrus.Logger, metrics *rabbitmq.SimpleMetrics) {
	logger.WithFields(logrus.Fields{
		"published": metrics.GetPublished(),
		"failed":    metrics.GetPublishFailed(),
		"retries":   metrics.GetPublishRetry(),
	}).Info("Producer metrics")
}
