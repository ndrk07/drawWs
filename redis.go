package main

import (
	"context"
	"fmt"

	redis "github.com/redis/go-redis/v9"
)

type Config struct {
	Addr     string
	Password string
	DB       int
}

func NewClient(ctx context.Context, cfg Config) (*redis.Client, error) {
	db := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := db.Ping(ctx).Err(); err != nil {
		fmt.Println(err)
		return nil, err
	}
	return db, nil
}
