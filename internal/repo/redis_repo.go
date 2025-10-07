// internal/repo/repo.go
package repo

import (
	"context"
	"log"
	"os"

	"github.com/redis/go-redis/v9"
)

type RedisRepo struct {
	rdb *redis.Client
}

func NewRedisClient() *RedisRepo {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Println("⚠️ REDIS_URL não definida no .env, usando fallback padrão redis://localhost:6379")
		redisURL = "redis://localhost:6379"
	}

	log.Printf("🔌 Conectando ao Redis com URL: %s", redisURL)

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("❌ Erro ao fazer parse da URL do Redis: %v", err)
	}

	client := redis.NewClient(opts)

	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("❌ Erro ao conectar no Redis: %v", err)
	}

	log.Println("✅ Redis conectado com sucesso!")
	return &RedisRepo{rdb: client}
}

func (r *RedisRepo) Ping(ctx context.Context) error { return r.rdb.Ping(ctx).Err() }
func (r *RedisRepo) Rdb() *redis.Client             { return r.rdb }


