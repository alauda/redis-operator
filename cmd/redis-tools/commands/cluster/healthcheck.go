package cluster

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/alauda/redis-operator/pkg/redis"
)

func Readiness(ctx context.Context, addr string, authInfo redis.AuthInfo) error {
	client := redis.NewRedisClient(addr, authInfo)
	defer client.Close()

	nodes, err := client.Nodes(ctx)
	if err != nil {
		return err
	}
	self := nodes.Self()
	if self == nil {
		return errors.New("self node not found")
	}
	if !self.IsJoined() {
		return errors.New("node is not joined")
	}
	if self.IsFailed() {
		return errors.New("node is failed")
	}
	return nil
}

// Ping
func Ping(ctx context.Context, addr string, authInfo redis.AuthInfo) error {
	client := redis.NewRedisClient(addr, authInfo)
	defer client.Close()

	if _, err := client.Do(ctx, "PING"); err != nil {
		return err
	}
	return nil
}

// TcpSocket
func TcpSocket(ctx context.Context, addr string, t time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, t)
	if err != nil {
		return err
	}
	conn.Close()

	return nil
}
