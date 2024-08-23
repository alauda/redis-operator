package helper

import (
	"context"

	"github.com/alauda/redis-operator/pkg/redis"
)

// Ping
func Ping(ctx context.Context, addr string, authInfo redis.AuthInfo) error {
	client := redis.NewRedisClient(addr, authInfo)
	defer client.Close()

	if _, err := client.Do(ctx, "PING"); err != nil {
		return err
	}
	return nil
}
