// Package natsjs provides a NATS JetStream queue. It does not store jobs.
package natsjs

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/snowmerak/transliter/lib/jobs"
)

type Config struct {
	URL        string
	Stream     string
	Subject    string
	Consumer   string
	AckWait    time.Duration
	MaxDeliver int
	Storage    jetstream.StorageType
}

type Queue struct {
	connection *nats.Conn
	stream     jetstream.JetStream
	consumer   jetstream.Consumer
	subject    string
	ownsConn   bool
}

var _ jobs.Queue = (*Queue)(nil)

func New(ctx context.Context, config Config, options ...nats.Option) (*Queue, error) {
	rawURL := config.URL
	if rawURL == "" {
		rawURL = nats.DefaultURL
	}
	connection, err := nats.Connect(rawURL, options...)
	if err != nil {
		return nil, fmt.Errorf("connect NATS: %w", err)
	}
	queue, err := NewWithConnection(ctx, connection, config)
	if err != nil {
		connection.Close()
		return nil, err
	}
	queue.ownsConn = true
	return queue, nil
}

func NewWithConnection(
	ctx context.Context,
	connection *nats.Conn,
	config Config,
) (*Queue, error) {
	if connection == nil {
		return nil, fmt.Errorf("NATS connection is required")
	}
	if config.Stream == "" {
		config.Stream = "TRANSLITER_JOBS"
	}
	if config.Subject == "" {
		config.Subject = "transliter.jobs"
	}
	if config.Consumer == "" {
		config.Consumer = "transliter-workers"
	}
	if config.AckWait <= 0 {
		config.AckWait = 10 * time.Minute
	}
	if config.MaxDeliver <= 0 {
		config.MaxDeliver = 20
	}

	js, err := jetstream.New(connection)
	if err != nil {
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      config.Stream,
		Subjects:  []string{config.Subject},
		Storage:   config.Storage,
		Retention: jetstream.WorkQueuePolicy,
	}); err != nil {
		return nil, fmt.Errorf("create JetStream stream: %w", err)
	}
	consumer, err := js.CreateOrUpdateConsumer(ctx, config.Stream, jetstream.ConsumerConfig{
		Durable:       config.Consumer,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       config.AckWait,
		MaxDeliver:    config.MaxDeliver,
		FilterSubject: config.Subject,
	})
	if err != nil {
		return nil, fmt.Errorf("create JetStream consumer: %w", err)
	}
	return &Queue{
		connection: connection,
		stream:     js,
		consumer:   consumer,
		subject:    config.Subject,
	}, nil
}

func (queue *Queue) Close() {
	if queue.ownsConn {
		queue.connection.Close()
	}
}

func (queue *Queue) Enqueue(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("job ID must not be empty")
	}
	_, err := queue.stream.Publish(
		ctx,
		queue.subject,
		[]byte(jobID),
		jetstream.WithMsgID(jobID),
	)
	return err
}

func (queue *Queue) Receive(ctx context.Context) (jobs.Delivery, error) {
	batch, err := queue.consumer.Fetch(1, jetstream.FetchContext(ctx))
	if err != nil {
		return nil, err
	}
	for message := range batch.Messages() {
		jobID := string(message.Data())
		if jobID == "" {
			_ = message.Term()
			continue
		}
		return &delivery{message: message, jobID: jobID}, nil
	}
	if err := batch.Error(); err != nil {
		return nil, err
	}
	return nil, ctx.Err()
}

type delivery struct {
	message jetstream.Msg
	jobID   string
}

func (value *delivery) JobID() string {
	return value.jobID
}

func (value *delivery) Ack(ctx context.Context) error {
	return value.message.DoubleAck(ctx)
}

func (value *delivery) Nack(context.Context) error {
	return value.message.Nak()
}
