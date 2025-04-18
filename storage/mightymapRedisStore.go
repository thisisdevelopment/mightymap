package storage

import (
	"context"
	"crypto/tls"
	"strings"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	msgpack "github.com/vmihailenco/msgpack/v5"
)

type mightyMapRedisStorage[K comparable, V any] struct {
	redisClient *redis.Client
	opts        *redisOpts
}

func NewMightyMapRedisStorage[K comparable, V any](optfuncs ...OptionFuncRedis) IMightyMapStorage[K, V] {
	opts := getDefaultRedisOptions()
	for _, optfunc := range optfuncs {
		optfunc(opts)
	}
	if opts.tlsConfig == nil && opts.tls {
		opts.tlsConfig = &tls.Config{}
	}

	clientOpts := &redis.Options{
		Addr:       opts.addr,
		Password:   opts.password,
		DB:         opts.db,
		PoolSize:   opts.poolSize,
		MaxRetries: opts.maxRetries,
	}

	if opts.tls {
		clientOpts.TLSConfig = opts.tlsConfig
	}

	if opts.mock != nil {
		mr := miniredis.RunT(opts.mock)
		clientOpts = &redis.Options{
			Addr: mr.Addr(),
		}
	}

	return &mightyMapRedisStorage[K, V]{
		redisClient: redis.NewClient(clientOpts),
		opts:        opts,
	}
}

func getDefaultRedisOptions() *redisOpts {
	opts := &redisOpts{
		addr:       "localhost:6379",
		password:   "",
		db:         0,
		poolSize:   3,
		maxRetries: 3,
		tls:        false,
		tlsConfig:  nil,
		prefix:     "mightymap_",
		timeout:    5 * time.Second,
		expire:     0,
	}

	return opts
}

func (c *mightyMapRedisStorage[K, V]) Store(ctx context.Context, key K, value V) {
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
	defer cancel()

	valueBytes, err := msgpack.Marshal(value)
	if err != nil {
		panic(err)
	}

	if err := c.redisClient.Set(ctx, c.opts.prefix+string(keyBytes), valueBytes, c.opts.expire).Err(); err != nil {
		panic(err)
	}
}

func (c *mightyMapRedisStorage[K, V]) Load(ctx context.Context, key K) (value V, ok bool) {
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
	defer cancel()

	v, err := c.redisClient.Get(ctx, c.opts.prefix+string(keyBytes)).Bytes()
	var zeroV V
	if err == redis.Nil {
		return zeroV, false
	}
	if err != nil {
		panic(err)
	}

	err = msgpack.Unmarshal(v, &value)
	if err != nil {
		panic(err)
	}
	return value, true
}

func (c *mightyMapRedisStorage[K, V]) Delete(ctx context.Context, keys ...K) {
	for _, key := range keys {
		keyBytes, err := msgpack.Marshal(key)
		if err != nil {
			panic(err)
		}
		ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
		defer cancel()
		if err := c.redisClient.Del(ctx, c.opts.prefix+string(keyBytes)).Err(); err != nil {
			panic(err)
		}
	}
}

func (c *mightyMapRedisStorage[K, V]) Clear(ctx context.Context) {
	keys, err := c.scan(ctx, c.opts.prefix+"*")
	if err != nil {
		panic(err)
	}

	var kkeys []K
	for _, key := range keys {
		keySplit := strings.SplitN(key, c.opts.prefix, 2)
		if len(keySplit) != 2 {
			continue
		}
		var k K
		err := msgpack.Unmarshal([]byte(keySplit[1]), &k)
		if err != nil {
			panic(err)
		}
		kkeys = append(kkeys, k)
	}

	if len(kkeys) > 0 {
		c.Delete(ctx, kkeys...)
	}
}

func (c *mightyMapRedisStorage[K, V]) Close(_ context.Context) error {
	return c.redisClient.Close()
}

func (c *mightyMapRedisStorage[K, V]) Len(ctx context.Context) int {
	keys, err := c.scan(ctx, c.opts.prefix+"*")
	if err != nil {
		panic(err)
	}
	return len(keys)
}

func (c *mightyMapRedisStorage[K, V]) Next(ctx context.Context) (key K, value V, ok bool) {
	var zeroV V
	var zeroK K

	ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
	defer cancel()

	keys, err := c.scan(ctx, c.opts.prefix+"*", 1)
	if err != nil {
		panic(err)
	}
	if len(keys) == 0 {
		return zeroK, zeroV, false
	}

	splitKey := strings.SplitN(keys[0], c.opts.prefix, 2)
	if len(splitKey) != 2 {
		return zeroK, zeroV, false
	}

	var k K
	err = msgpack.Unmarshal([]byte(splitKey[1]), &k)
	if err != nil {
		panic(err)
	}
	v, err := c.redisClient.Get(ctx, keys[0]).Result()
	if err != nil {
		panic(err)
	}

	err = msgpack.Unmarshal([]byte(v), &value)
	if err != nil {
		panic(err)
	}

	c.Delete(ctx, k)

	return k, value, true
}

func (c *mightyMapRedisStorage[K, V]) Range(ctx context.Context, f func(key K, value V) bool) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
	defer cancel()

	keys, err := c.scan(ctx, c.opts.prefix+"*")
	if err != nil {
		panic(err)
	}
	for _, key := range keys {
		splitKey := strings.SplitN(key, c.opts.prefix, 2)
		if len(splitKey) != 2 {
			continue
		}

		vb, err := c.redisClient.Get(ctx, key).Result()
		if err != nil {
			panic(err)
		}

		var k K
		err = msgpack.Unmarshal([]byte(splitKey[1]), &k)
		if err != nil {
			panic(err)
		}

		var v V
		err = msgpack.Unmarshal([]byte(vb), &v)
		if err != nil {
			panic(err)
		}

		if !f(k, v) {
			break
		}
	}
}

const defaultCursorSize int64 = 2048

func (c *mightyMapRedisStorage[K, V]) scan(ctx context.Context, keyPattern string, maxKeys ...int) ([]string, error) {
	max := defaultCursorSize
	if len(maxKeys) > 0 {
		max = int64(maxKeys[0])
	}

	var (
		cursor uint64
		keys   []string
		err    error
		kk     []string
	)

	ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
	defer cancel()

	for {

		// only string keys are returned no payloads
		// this might be a lot slower on elasicache
		kk, cursor, err = c.redisClient.Scan(ctx, cursor, keyPattern, max).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, kk...)

		if cursor == 0 {
			break
		}
	}
	return keys, nil
}
