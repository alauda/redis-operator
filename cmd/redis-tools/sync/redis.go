package sync

import (
	"errors"
)

type RedisClusterFilter struct{}

func (v *RedisClusterFilter) Truncate(filename string, data string) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty data")
	}
	return data, nil
}
