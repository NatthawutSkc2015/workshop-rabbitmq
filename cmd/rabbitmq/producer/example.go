package main

import (
	"context"
	"encoding/json"
	"fmt"
	config "go-queue/configs/rabbitmq"
	"go-queue/pkg/rabbitmq"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/sirupsen/logrus"
)

func init() {

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

	// Publish loop
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	messageCount := 0
	for {
		select {
		case <-ctx.Done():
			logger.Info("Producer stopped")
			printMetrics(logger, metrics)
			return
		case <-ticker.C:
			messageCount++
			msg := Message{
				ID:        fmt.Sprintf("msg-%d", messageCount),
				Content:   fmt.Sprintf("Hello RabbitMQ! Message #%d", messageCount),
				Timestamp: time.Now(),
			}

			body, err := json.Marshal(msg)
			if err != nil {
				logger.WithError(err).Error("Failed to marshal message")
				continue
			}

			headers := amqp.Table{
				"x-message-id": msg.ID,
			}

			logger.WithFields(logrus.Fields{
				"message_id":  msg.ID,
				"exchange":    cfg.Exchange,
				"routing_key": cfg.RoutingKey,
			}).Info("Publishing message")

			if err := producer.Publish(ctx, cfg.Exchange, cfg.RoutingKey, body, headers); err != nil {
				logger.WithError(err).Error("Failed to publish message")
			} else {
				logger.WithField("message_id", msg.ID).Info("Message published successfully")
			}

			// Print metrics every 10 messages
			if messageCount%10 == 0 {
				printMetrics(logger, metrics)
			}
		}
	}

}
