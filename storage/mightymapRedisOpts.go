package storage

import (
	"crypto/tls"
	"testing"
	"time"
)

type redisOpts struct {
	addr       string
	password   string
	db         int
	poolSize   int
	maxRetries int
	tls        bool
	tlsConfig  *tls.Config
	prefix     string
	timeout    time.Duration
	expire     time.Duration
	mock       *testing.T
}

type OptionFuncRedis func(*redisOpts)

// WithRedisExpire sets the expiration time for the Redis key-value pairs.
// The expire parameter specifies the duration after which keys will expire.
// If expire is 0, keys will not expire (persist indefinitely).
func WithRedisExpire(expire time.Duration) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.expire = expire
	}
}

// WithRedisTimeout sets the timeout duration for Redis client operations.
// This timeout value is used to create a context with timeout for Redis operations.
// It helps prevent operations from hanging indefinitely.
func WithRedisTimeout(timeout time.Duration) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.timeout = timeout
	}
}

// WithRedisAddr sets the Redis server address (host:port) for the client connection.
// Example: "localhost:6379" or "redis.example.com:6379"
func WithRedisAddr(addr string) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.addr = addr
	}
}

// WithRedisPassword sets the password for Redis authentication.
// If Redis server requires authentication, this password will be used.
// For servers without authentication, pass an empty string.
func WithRedisPassword(password string) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.password = password
	}
}

// WithRedisDB selects the Redis logical database to use.
// Redis servers support multiple logical databases indexed by numbers (0-15 by default).
// The default database is 0.
func WithRedisDB(db int) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.db = db
	}
}

// WithRedisPoolSize sets the maximum number of socket connections in the Redis connection pool.
// Default is 10 connections per every available CPU as reported by runtime.NumCPU.
func WithRedisPoolSize(poolSize int) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.poolSize = poolSize
	}
}

// WithRedisMaxRetries sets the maximum number of retries for failed Redis operations.
// The client will retry operations if they fail due to network issues or other recoverable errors.
func WithRedisMaxRetries(maxRetries int) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.maxRetries = maxRetries
	}
}

// WithRedisTLS enables or disables TLS/SSL encryption for Redis connections.
// When enabled, the client will attempt to establish secure connections to the Redis server.
func WithRedisTLS(tls bool) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.tls = tls
	}
}

// WithRedisTLSConfig sets custom TLS configuration for Redis connections.
// This allows fine-grained control over TLS settings such as certificates, cipher suites, etc.
// Only used when TLS is enabled.
func WithRedisTLSConfig(tlsConfig *tls.Config) OptionFuncRedis {
	return func(opts *redisOpts) {
		if opts.tlsConfig == nil {
			opts.tlsConfig = &tls.Config{}
			return
		}
		opts.tlsConfig = tlsConfig
	}
}

// WithRedisPrefix sets a global key prefix for all Redis operations.
// All keys will be automatically prefixed with this string.
// Useful for namespacing keys in a shared Redis instance.
func WithRedisPrefix(prefix string) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.prefix = prefix
	}
}

// WithRedisMock (not implemented) sets the Redis client to use a mock implementation.
// This is useful for testing and development environments where a real Redis server is not available.
func WithRedisMock(t *testing.T) OptionFuncRedis {
	return func(opts *redisOpts) {
		opts.mock = t
	}
}
