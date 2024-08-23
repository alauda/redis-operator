package util

import (
	"crypto/tls"
)

func LoadTLSConfig(tlsKeyFile, tlsCertFile string, skipverify bool) (*tls.Config, error) {
	if tlsKeyFile != "" && tlsCertFile != "" {
		cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
		if err != nil {
			return nil, err
		}
		return &tls.Config{
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: skipverify, // #nosec G402
		}, nil
	}
	return nil, nil
}
