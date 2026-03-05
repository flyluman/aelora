package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/valkey"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Containers struct {
	PostgresDSN string
	ValkeyAddr  string
	Cleanup     func()
}

func StartContainers(ctx context.Context) (*Containers, error) {
	pg, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgres.WithDatabase("aelora"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("pg dsn: %w", err)
	}

	vk, err := valkey.RunContainer(ctx,
		testcontainers.WithImage("valkey/valkey:8-alpine"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("* Ready to accept connections")),
	)
	if err != nil {
		return nil, fmt.Errorf("valkey: %w", err)
	}

	addr, err := vk.Endpoint(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("valkey addr: %w", err)
	}

	return &Containers{
		PostgresDSN: dsn,
		ValkeyAddr:  addr,
		Cleanup: func() {
			_ = pg.Terminate(ctx)
			_ = vk.Terminate(ctx)
		},
	}, nil
}
