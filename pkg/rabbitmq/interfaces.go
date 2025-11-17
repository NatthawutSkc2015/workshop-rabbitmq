package rabbitmq

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPConnection abstracts RabbitMQ connection for testing
type AMQPConnection interface {
	Channel() (AMQPChannel, error)
	Close() error
	NotifyClose(receiver chan *amqp.Error) chan *amqp.Error
	IsClosed() bool
}

// AMQPChannel abstracts RabbitMQ channel for testing
type AMQPChannel interface {
	Qos(prefetchCount, prefetchSize int, global bool) error
	Confirm(noWait bool) error
	NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation
	NotifyReturn(returns chan amqp.Return) chan amqp.Return
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	Close() error
	NotifyClose(receiver chan *amqp.Error) chan *amqp.Error
}

// ConnectionFactory creates RabbitMQ connections
type ConnectionFactory interface {
	Dial(url string) (AMQPConnection, error)
}

// RealConnection wraps amqp.Connection
type RealConnection struct {
	conn *amqp.Connection
}

func (r *RealConnection) Channel() (AMQPChannel, error) {
	ch, err := r.conn.Channel()
	if err != nil {
		return nil, err
	}
	return &RealChannel{ch: ch}, nil
}

func (r *RealConnection) Close() error {
	return r.conn.Close()
}

func (r *RealConnection) NotifyClose(receiver chan *amqp.Error) chan *amqp.Error {
	return r.conn.NotifyClose(receiver)
}

func (r *RealConnection) IsClosed() bool {
	return r.conn.IsClosed()
}

// RealChannel wraps amqp.Channel
type RealChannel struct {
	ch *amqp.Channel
}

func (r *RealChannel) Qos(prefetchCount, prefetchSize int, global bool) error {
	return r.ch.Qos(prefetchCount, prefetchSize, global)
}

func (r *RealChannel) Confirm(noWait bool) error {
	return r.ch.Confirm(noWait)
}

func (r *RealChannel) NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation {
	return r.ch.NotifyPublish(confirm)
}

func (r *RealChannel) NotifyReturn(returns chan amqp.Return) chan amqp.Return {
	return r.ch.NotifyReturn(returns)
}

func (r *RealChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	return r.ch.PublishWithContext(ctx, exchange, key, mandatory, immediate, msg)
}

func (r *RealChannel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	return r.ch.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}

func (r *RealChannel) Close() error {
	return r.ch.Close()
}

func (r *RealChannel) NotifyClose(receiver chan *amqp.Error) chan *amqp.Error {
	return r.ch.NotifyClose(receiver)
}

// RealConnectionFactory creates real AMQP connections
type RealConnectionFactory struct{}

func (f *RealConnectionFactory) Dial(url string) (AMQPConnection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	return &RealConnection{conn: conn}, nil
}
