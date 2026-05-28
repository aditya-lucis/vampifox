// Package shadow — Cache layer VampiFox menggunakan Redis.
package shadow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/aditya-lucis/vampifox/internal/config"
)

var (
	ErrNotFound = errors.New("shadow: key tidak ditemukan di cache")
	ErrNilValue = errors.New("shadow: tidak bisa menyimpan nil value")
)

type Shadow struct {
	client    *redis.Client
	cfg       config.ShadowConfig
	logger    *zap.Logger
	globalPfx string
}

func New(cfg config.ShadowConfig, logger *zap.Logger) (*Shadow, error) {
	if logger == nil {
		return nil, fmt.Errorf("[Shadow] logger tidak boleh nil")
	}
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("[Shadow] Redis tidak merespons di %s: %w", cfg.Addr, err)
	}
	logger.Info("Shadow terhubung", zap.String("addr", cfg.Addr))
	return &Shadow{client: client, cfg: cfg, logger: logger, globalPfx: "vfx:"}, nil
}

func (s *Shadow) ForTenant(tenantSlug string) *TenantShadow {
	if tenantSlug == "" {
		s.logger.Warn("[Shadow] ForTenant dipanggil dengan slug kosong")
	}
	return &TenantShadow{shadow: s, ns: s.globalPfx + tenantSlug + ":", slug: tenantSlug}
}

func (s *Shadow) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *Shadow) Stats() *redis.PoolStats { return s.client.PoolStats() }

func (s *Shadow) Close() error {
	if err := s.client.Close(); err != nil {
		return fmt.Errorf("[Shadow] gagal menutup koneksi Redis: %w", err)
	}
	s.logger.Info("Shadow ditutup")
	return nil
}

// TenantShadow — operasi cache per-tenant
type TenantShadow struct {
	shadow *Shadow
	ns     string
	slug   string
}

func (ts *TenantShadow) k(key string) string { return ts.ns + key }

func (ts *TenantShadow) Haunt(ctx context.Context, key string, val any, ttl time.Duration) error {
	if val == nil {
		return ErrNilValue
	}
	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("[Shadow] gagal marshal key '%s': %w", key, err)
	}
	return ts.shadow.client.Set(ctx, ts.k(key), b, ttl).Err()
}

func (ts *TenantShadow) HauntNX(ctx context.Context, key string, val any, ttl time.Duration) (bool, error) {
	if val == nil {
		return false, ErrNilValue
	}
	b, err := json.Marshal(val)
	if err != nil {
		return false, err
	}
	return ts.shadow.client.SetNX(ctx, ts.k(key), b, ttl).Result()
}

func (ts *TenantShadow) Recall(ctx context.Context, key string, dest any) error {
	b, err := ts.shadow.client.Get(ctx, ts.k(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrNotFound
		}
		return fmt.Errorf("[Shadow] gagal Recall key '%s': %w", key, err)
	}
	return json.Unmarshal(b, dest)
}

func (ts *TenantShadow) RecallOrSet(ctx context.Context, key string, ttl time.Duration, fn func() (any, error), dest any) error {
	err := ts.Recall(ctx, key, dest)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	val, err := fn()
	if err != nil {
		return err
	}
	if haunErr := ts.Haunt(ctx, key, val, ttl); haunErr != nil {
		ts.shadow.logger.Warn("Gagal Haunt setelah RecallOrSet", zap.String("key", key), zap.Error(haunErr))
	}
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func (ts *TenantShadow) Vanish(ctx context.Context, key string) error {
	return ts.shadow.client.Del(ctx, ts.k(key)).Err()
}

func (ts *TenantShadow) VanishMany(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	fullKeys := make([]string, len(keys))
	for i, k := range keys {
		fullKeys[i] = ts.k(k)
	}
	return ts.shadow.client.Del(ctx, fullKeys...).Err()
}

func (ts *TenantShadow) Dispel(ctx context.Context, pattern string) (int64, error) {
	fullPattern := ts.k(pattern)
	var deleted int64
	var cursor uint64
	for {
		keys, nextCursor, err := ts.shadow.client.Scan(ctx, cursor, fullPattern, 100).Result()
		if err != nil {
			return deleted, err
		}
		if len(keys) > 0 {
			n, err := ts.shadow.client.Del(ctx, keys...).Result()
			if err != nil {
				return deleted, err
			}
			deleted += n
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return deleted, nil
}

func (ts *TenantShadow) Exists(ctx context.Context, key string) (bool, error) {
	n, err := ts.shadow.client.Exists(ctx, ts.k(key)).Result()
	return n > 0, err
}

func (ts *TenantShadow) TTL(ctx context.Context, key string) (time.Duration, error) {
	return ts.shadow.client.TTL(ctx, ts.k(key)).Result()
}

func (ts *TenantShadow) Refresh(ctx context.Context, key string, ttl time.Duration) error {
	ok, err := ts.shadow.client.Expire(ctx, ts.k(key), ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (ts *TenantShadow) Increment(ctx context.Context, key string) (int64, error) {
	return ts.shadow.client.Incr(ctx, ts.k(key)).Result()
}

func (ts *TenantShadow) IncrementBy(ctx context.Context, key string, n int64) (int64, error) {
	return ts.shadow.client.IncrBy(ctx, ts.k(key), n).Result()
}

func (ts *TenantShadow) FlushTenant(ctx context.Context) (int64, error) {
	ts.shadow.logger.Warn("FlushTenant dipanggil", zap.String("tenant", ts.slug))
	return ts.Dispel(ctx, "*")
}

func (ts *TenantShadow) Namespace() string { return ts.ns }
func (ts *TenantShadow) Slug() string      { return ts.slug }

func (ts *TenantShadow) Pipeline() redis.Pipeliner { return ts.shadow.client.Pipeline() }
func (ts *TenantShadow) RawKey(key string) string  { return ts.k(key) }

func BuildKey(parts ...string) string { return strings.Join(parts, ":") }
