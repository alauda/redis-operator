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
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
)

// RedisInfo
type RedisInfo struct {
	RedisVersion          string             `json:"redis_version"`
	RedisMode             string             `json:"redis_mode"`
	RunId                 string             `json:"run_id"`
	UptimeInSeconds       string             `json:"uptime_in_seconds"`
	AOFEnabled            string             `json:"aof_enabled"`
	Role                  string             `json:"role"`
	MasterHost            string             `json:"master_host"`
	MasterPort            string             `json:"master_port"`
	ClusterEnabled        string             `json:"cluster_enabled"`
	MasterLinkStatus      string             `json:"master_link_status"`
	MasterReplOffset      int64              `json:"master_repl_offset"`
	UsedMemoryDataset     int64              `json:"used_memory_dataset"`
	SentinelMasters       int64              `json:"sentinel_masters"`
	SentinelTiLt          int64              `json:"sentinel_tilt"`
	SentinelRunningScript int64              `json:"sentinel_running_scripts"`
	SentinelMaster0       SentinelMasterInfo `json:"master0"`
	ConnectedReplicas     int64              `json:"connected_slaves"`
}

type SentinelMasterInfo struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	Address   Address `json:"address"`
	Replicas  int     `json:"slaves"`
	Sentinels int     `json:"sentinels"`
}

func parseIPAndPort(ipPortString string) (net.IP, int, error) {
	// 查找最后一个冒号，将其之前的部分作为IP地址，之后的部分作为端口号
	lastColonIndex := strings.LastIndex(ipPortString, ":")
	if lastColonIndex == -1 {
		return nil, 0, fmt.Errorf("Invalid IP:Port format")
	}

	ipStr := ipPortString[:lastColonIndex]
	portStr := ipPortString[lastColonIndex+1:]

	// 解析IP地址
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, 0, fmt.Errorf("Invalid IP address")
	}

	// 解析端口号
	portNumber, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("[%s]:%s", ip.String(), portStr))
	if err != nil {
		return nil, 0, fmt.Errorf("Invalid port number: %v", err)
	}

	return ip, portNumber.Port, nil
}

type Address string

func (a Address) Host() string {
	address, _, err := parseIPAndPort(string(a))
	if err != nil {
		return ""
	}
	return address.String()
}

func (a Address) Port() int {
	_, port, err := parseIPAndPort(string(a))
	if err != nil {
		return 0
	}
	return port
}

func (a Address) ToString() string {
	return string(a)
}

// RedisClient
type RedisClient interface {
	Do(ctx context.Context, cmd string, args ...any) (any, error)
	Pipelining(ctx context.Context, cmds []string, args [][]any) (interface{}, error)
	Close() error

	Ping(ctx context.Context) error
	Info(ctx context.Context) (*RedisInfo, error)
	ConfigGet(ctx context.Context, cate string) (map[string]string, error)
	ConfigSet(ctx context.Context, params map[string]string) error
	Nodes(ctx context.Context) (ClusterNodes, error)
}

type AuthConfig struct {
	Username  string
	Password  string
	TLSConfig *tls.Config
}

type redisClient struct {
	authInfo *AuthConfig
	addr     string

	pool *redis.Pool
}

// NewRedisClient
func NewRedisClient(addr string, authInfo AuthConfig) RedisClient {
	client := redisClient{authInfo: &authInfo, addr: addr}
	client.pool = &redis.Pool{
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			var opts []redis.DialOption
			if authInfo.Password != "" {
				if authInfo.Username != "" && authInfo.Username != "default" {
					opts = append(opts, redis.DialUsername(authInfo.Username))
				}
				opts = append(opts, redis.DialPassword(authInfo.Password))
			}
			if authInfo.TLSConfig != nil {
				opts = append(opts,
					redis.DialUseTLS(true),
					redis.DialTLSConfig(authInfo.TLSConfig),
					redis.DialTLSSkipVerify(true),
				)
			}

			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()

			conn, err := redis.DialContext(ctx, "tcp", addr, opts...)
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
func (c *redisClient) Do(ctx context.Context, cmd string, args ...any) (any, error) {
	if c == nil || c.pool == nil {
		return nil, nil
	}

	conn := c.pool.Get()
	defer conn.Close()

	return redis.DoContext(conn, ctx, cmd, args...)
}

// Pipeline
func (c *redisClient) Pipelining(ctx context.Context, cmds []string, args [][]any) (interface{}, error) {
	if c == nil || c.pool == nil {
		return nil, nil
	}

	conn := c.pool.Get()
	defer conn.Close()

	if err := conn.Send("MULTI"); err != nil {
		return nil, err
	}
	for i, cmd := range cmds {
		if err := conn.Send(cmd, args[i]...); err != nil {
			return nil, err
		}
	}

	result, err := redis.Values(conn.Do("EXEC"))
	if err != nil {
		return result, err
	}
	for _, v := range result {
		switch v.(type) {
		case redis.Error:
			return result, fmt.Errorf("redis error: %s", v)
		}
	}
	return result, nil
}

// Close
func (c *redisClient) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// Ping
func (c *redisClient) Ping(ctx context.Context) error {
	if c == nil || c.pool == nil {
		return nil
	}

	conn := c.pool.Get()
	defer conn.Close()

	if _, err := redis.DoContext(conn, ctx, "PING"); err != nil {
		return err
	}
	return nil
}

var (
	ErrNil     = redis.ErrNil
	Int        = redis.Int
	Ints       = redis.Ints
	IntMap     = redis.IntMap
	Int64      = redis.Int64
	Int64s     = redis.Int64s
	Int64Map   = redis.Int64Map
	Uint64     = redis.Uint64
	Uint64s    = redis.Uint64s
	Uint64Map  = redis.Uint64Map
	Float64    = redis.Float64
	Float64s   = redis.Float64s
	Float64Map = redis.Float64Map
	String     = redis.String
	Strings    = redis.Strings
	StringMap  = redis.StringMap
	Bytes      = redis.Bytes
	ByteSlices = redis.ByteSlices
	Bool       = redis.Bool
	Values     = redis.Values
	Positions  = redis.Positions
)

// Config
func (c *redisClient) ConfigGet(ctx context.Context, cate string) (map[string]string, error) {
	if c == nil || c.pool == nil {
		return nil, nil
	}

	if cate == "" {
		cate = "*"
	}

	conn := c.pool.Get()
	defer conn.Close()

	return redis.StringMap(redis.DoContext(conn, ctx, "CONFIG", "GET", cate))
}

// ConfigSet
func (c *redisClient) ConfigSet(ctx context.Context, params map[string]string) error {
	if c == nil || c.pool == nil {
		return nil
	}

	conn := c.pool.Get()
	defer conn.Close()

	for key, val := range params {
		if val == `""` {
			val = ""
		}
		if _, err := redis.DoContext(conn, ctx, "CONFIG", "SET", key, val); err != nil {
			return fmt.Errorf("update config %s failed, error=%s", key, err)
		}
	}
	return nil
}

// Info
func (c *redisClient) Info(ctx context.Context) (*RedisInfo, error) {
	if c == nil || c.pool == nil {
		return nil, nil
	}

	conn := c.pool.Get()
	defer conn.Close()

	data, err := redis.String(redis.DoContext(conn, ctx, "INFO"))
	if err != nil {
		return nil, err
	}

	parseInfo := func(data string) *RedisInfo {
		info := RedisInfo{}
		lines := strings.Split(data, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.SplitN(line, ":", 2)
			if len(fields) != 2 {
				continue
			}
			switch fields[0] {
			case "redis_version":
				info.RedisVersion = fields[1]
			case "redis_mode":
				info.RedisMode = fields[1]
			case "run_id":
				info.RunId = fields[1]
			case "uptime_in_seconds":
				info.UptimeInSeconds = fields[1]
			case "aof_enabled":
				info.AOFEnabled = fields[1]
			case "role":
				info.Role = fields[1]
			case "master_host":
				info.MasterHost = fields[1]
			case "master_port":
				info.MasterPort = fields[1]
			case "cluster_enabled":
				info.ClusterEnabled = fields[1]
			case "master_link_status":
				info.MasterLinkStatus = fields[1]
			case "master_repl_offset":
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				info.MasterReplOffset = val
			case "used_memory_dataset":
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				info.UsedMemoryDataset = val
			case "sentinel_masters":
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				info.SentinelMasters = val
			case "sentinel_tilt":
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				info.SentinelTiLt = val
			case "connected_slaves":
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				info.ConnectedReplicas = val
			case "sentinel_running_scripts":
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				info.SentinelRunningScript = val
			case "master0":
				fields := strings.Split(fields[1], ",")
				if len(fields) != 5 {
					continue
				}
				for _, kv := range fields {
					kv := strings.Split(kv, "=")
					if len(kv) != 2 {
						continue
					}
					key, value := kv[0], kv[1]
					switch key {
					case "name":
						info.SentinelMaster0.Name = value
					case "status":
						info.SentinelMaster0.Status = value
					case "address":
						info.SentinelMaster0.Address = Address(value)
					case "slaves":
						info.SentinelMaster0.Replicas, _ = strconv.Atoi(value)
					case "sentinels":
						info.SentinelMaster0.Sentinels, _ = strconv.Atoi(value)
					}
				}
			}
		}
		return &info
	}
	return parseInfo(data), nil
}

// Nodes
func (c *redisClient) Nodes(ctx context.Context) (ClusterNodes, error) {
	if c == nil || c.pool == nil {
		return nil, nil
	}
	conn := c.pool.Get()
	defer conn.Close()

	data, err := redis.String(redis.DoContext(conn, ctx, "CLUSTER", "NODES"))
	if err != nil {
		return nil, err
	}

	var nodes ClusterNodes
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if node, err := ParseNodeFromClusterNode(line); err != nil {
			return nil, err
		} else {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}
