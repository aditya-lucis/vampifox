// Package shadow — Layer cache VampiFox menggunakan Redis.
// "Shadow" karena cache adalah bayangan dari data asli —
// selalu mengikuti, selalu lebih cepat.
package shadow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Shadow adalah cache client VampiFox.
type Shadow struct {
	client *redis.Client
	prefix string // namespace per-tenant, e.g. "vfx:tenant_abc:"
}

// ShadowConfig konfigurasi Redis.
type ShadowConfig struct {
	Addr     string
	Password string
	DB       int
	Prefix   string
}

// NewShadow membuat Shadow cache baru.
func NewShadow(cfg ShadowConfig) (*Shadow, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("shadow tidak bisa terhubung ke Redis: %w", err)
	}

	return &Shadow{
		client: client,
		prefix: cfg.Prefix,
	}, nil
}

// key menambahkan prefix tenant ke cache key.
func (s *Shadow) key(k string) string {
	return s.prefix + k
}

// Haunt menyimpan value ke cache — "menghantui" key tersebut.
func (s *Shadow) Haunt(
	ctx context.Context,
	key string,
	val any,
	ttl time.Duration,
) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}

	return s.client.Set(
		ctx,
		s.key(key),
		b,
		ttl,
	).Err()
}

// Recall mengambil value dari cache — "memanggil bayangan".
func (s *Shadow) Recall(
	ctx context.Context,
	key string,
	dest any,
) error {
	b, err := s.client.Get(
		ctx,
		s.key(key),
	).Bytes()

	if err != nil {
		return err // redis.Nil jika tidak ada
	}

	return json.Unmarshal(b, dest)
}

// Vanish menghapus key dari cache — "bayangan menghilang".
func (s *Shadow) Vanish(
	ctx context.Context,
	key string,
) error {
	return s.client.Del(
		ctx,
		s.key(key),
	).Err()
}

// Dispel menghapus semua key dengan pattern — "mengusir semua bayangan".
func (s *Shadow) Dispel(
	ctx context.Context,
	pattern string,
) error {
	keys, err := s.client.Keys(
		ctx,
		s.key(pattern)+"*",
	).Result()

	if err != nil || len(keys) == 0 {
		return err
	}

	return s.client.Del(
		ctx,
		keys...,
	).Err()
}
