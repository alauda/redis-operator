/*
Copyright 2023 The RedisOperator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package redis

import (
	"context"
	"crypto/tls"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
)

// alias
var (
	ErrNil           = redis.ErrNil
	ErrPoolExhausted = redis.ErrPoolExhausted
)

var (
	Bool       = redis.Bool
	ByteSlices = redis.ByteSlices
	Bytes      = redis.Bytes
	Float64    = redis.Float64
	Float64Map = redis.Float64Map
	Float64s   = redis.Float64s
	Int        = redis.Int
	IntMap     = redis.IntMap
	Ints       = redis.Ints
	Int64      = redis.Int64
	Int64Map   = redis.Int64Map
	Int64s     = redis.Int64s
	Uint64     = redis.Uint64
	Uint64Map  = redis.Uint64Map
	Uint64s    = redis.Uint64s
	Values     = redis.Values
	Positions  = redis.Positions
	Scan       = redis.Scan
	ScanSlice  = redis.ScanSlice
	ScanStruct = redis.ScanStruct
	String     = redis.String
	Strings    = redis.Strings
)

type Client interface {
	Nodes(ctx context.Context) (Nodes, error)
	Do(ctx context.Context, cmd string, args ...interface{}) (interface{}, error)
	Close() error
	Clone(ctx context.Context, addr string) Client
	CheckProxyInfo(ctx context.Context) error
}

type _RedisClient struct {
	pool     *redis.Pool
	authInfo *AuthInfo
}

type AuthInfo struct {
	Username string
	Password string
	TLSConf  *tls.Config
}

func ipv6ToURL(ipv6 string) string {

	// Split the address into IP and port parts
	parts := strings.Split(ipv6, ":")

	// Join the IP parts together with colons
	ip := strings.Join(parts[:len(parts)-1], ":")

	// Surround the IP address with brackets
	ip = "[" + ip + "]"

	// Combine the IP and port back together
	address := ip + ":" + parts[len(parts)-1]

	return address
}

// For the purpose of special handling the information returned by cluster nodes
func getAddress(address string) string {
	if strings.Contains(address, "]") {
		return address
	}
	if strings.Count(address, ":") > 1 {
		return ipv6ToURL(address)
	}
	return address

}

// NewClient
func NewClient(addr string, authInfo AuthInfo) Client {
	client := _RedisClient{authInfo: &authInfo}
	addr_formated := getAddress(addr)
	client.pool = &redis.Pool{
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			var opts []redis.DialOption
			if authInfo.Password != "" {
				if authInfo.Username != "" && authInfo.Username != "default" {
					opts = append(opts, redis.DialUsername(authInfo.Username))
				}
				opts = append(opts, redis.DialPassword(authInfo.Password))
			}
			if authInfo.TLSConf != nil {
				opts = append(opts,
					redis.DialUseTLS(true),
					redis.DialTLSConfig(authInfo.TLSConf),
					redis.DialTLSSkipVerify(true),
				)
			}

			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()

			conn, err := redis.DialContext(ctx, "tcp", addr_formated, opts...)
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
		MaxIdle:   1,
		MaxActive: 1,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			if _, err := redis.DoContext(c, ctx, "PING"); err != nil {
				return err
			}
			return nil
		},
	}
	return &client
}

// Do
func (c *_RedisClient) Do(ctx context.Context, cmd string, args ...interface{}) (interface{}, error) {
	if c == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return redis.DoContext(conn, ctx, cmd, args...)
}

func (c *_RedisClient) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

func (c *_RedisClient) Clone(ctx context.Context, addr string) Client {
	if c == nil || c.authInfo == nil {
		return nil
	}

	return NewClient(addr, *c.authInfo)
}

// Nodes
func (c *_RedisClient) Nodes(ctx context.Context) (Nodes, error) {
	if c == nil {
		return nil, nil
	}

	data, err := String(c.Do(ctx, "cluster", "nodes"))
	if err != nil {
		return nil, err
	}
	return ParseNodes(data), nil
}

func (c *_RedisClient) CheckProxyInfo(ctx context.Context) error {
	if c == nil {
		return nil
	}

	data, err := String(c.Do(ctx, "info"))
	if err != nil {
		return err
	}
	proxyinfo := ParseProxyInfo(data)
	if proxyinfo.FailCount >= 3 || proxyinfo.UnknownRole >= 3 {
		err := c.tryTestGet(ctx)
		if err != nil {
			return err
		}
	}
	return err
}

// 尝试get __ALAUDA_REDIS_PROXY_TEST_KEY__
func (c *_RedisClient) tryTestGet(ctx context.Context) error {
	if c == nil {
		return nil
	}
	_, err := String(c.Do(ctx, "get", "__ALAUDA_REDIS_PROXY_TEST_KEY__"))
	if err != nil {
		return err
	}
	return nil
}
