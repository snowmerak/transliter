package natsjs

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type EmbeddedConfig struct {
	Queue     Config
	StoreDir  string
	InMemory  bool
	ReadyWait time.Duration
}

type Embedded struct {
	Server *server.Server
	Queue  *Queue
	conn   *nats.Conn
}

// NewEmbedded starts an in-process NATS server with JetStream enabled.
//
// This is intended for single-process deployments and tests. External NATS is
// the durable multi-process option.
func NewEmbedded(ctx context.Context, config EmbeddedConfig) (*Embedded, error) {
	readyWait := config.ReadyWait
	if readyWait <= 0 {
		readyWait = 10 * time.Second
	}
	options := &server.Options{
		DontListen: true,
		JetStream:  true,
		StoreDir:   config.StoreDir,
		NoSigs:     true,
	}
	instance, err := server.NewServer(options)
	if err != nil {
		return nil, fmt.Errorf("create embedded NATS server: %w", err)
	}
	go instance.Start()
	if !instance.ReadyForConnections(readyWait) {
		instance.Shutdown()
		return nil, fmt.Errorf("embedded NATS server did not become ready")
	}
	connection, err := nats.Connect("", nats.InProcessServer(instance))
	if err != nil {
		instance.Shutdown()
		return nil, fmt.Errorf("connect embedded NATS server: %w", err)
	}
	if config.InMemory {
		config.Queue.Storage = jetstream.MemoryStorage
	} else {
		config.Queue.Storage = jetstream.FileStorage
	}
	queue, err := NewWithConnection(ctx, connection, config.Queue)
	if err != nil {
		connection.Close()
		instance.Shutdown()
		return nil, err
	}
	return &Embedded{
		Server: instance,
		Queue:  queue,
		conn:   connection,
	}, nil
}

func (embedded *Embedded) Close() {
	embedded.conn.Close()
	embedded.Server.Shutdown()
	embedded.Server.WaitForShutdown()
}
