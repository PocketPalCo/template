package server

import "context"

type SessionStorage interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
	GetAll(ctx context.Context) ([]struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}, error)
}
