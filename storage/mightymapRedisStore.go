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

type mightyMapRedisStorage[K comparable] struct {
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
		Username:   opts.username,
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

	storage := &mightyMapRedisStorage[K]{
		redisClient: redis.NewClient(clientOpts),
		opts:        opts,
	}
	return newMsgpackAdapter[K, V](storage)
}

func getDefaultRedisOptions() *redisOpts {
	opts := &redisOpts{
		addr:       "localhost:6379",
		username:   "",
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

func (c *mightyMapRedisStorage[K]) Store(ctx context.Context, key K, value []byte) {
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
	defer cancel()

	if err := c.redisClient.Set(ctx, c.opts.prefix+string(keyBytes), value, c.opts.expire).Err(); err != nil {
		panic(err)
	}
}

func (c *mightyMapRedisStorage[K]) Load(ctx context.Context, key K) (value []byte, ok bool) {
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
	defer cancel()

	v, err := c.redisClient.Get(ctx, c.opts.prefix+string(keyBytes)).Bytes()
	if err == redis.Nil {
		return nil, false
	}
	if err != nil {
		panic(err)
	}

	return v, true
}

func (c *mightyMapRedisStorage[K]) Delete(ctx context.Context, keys ...K) {
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

func (c *mightyMapRedisStorage[K]) Clear(ctx context.Context) {
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

func (c *mightyMapRedisStorage[K]) Close(_ context.Context) error {
	return c.redisClient.Close()
}

func (c *mightyMapRedisStorage[K]) Len(ctx context.Context) int {
	keys, err := c.scan(ctx, c.opts.prefix+"*")
	if err != nil {
		panic(err)
	}
	return len(keys)
}

func (c *mightyMapRedisStorage[K]) Next(ctx context.Context) (key K, value []byte, ok bool) {
	var zeroK K

	ctx, cancel := context.WithTimeout(ctx, c.opts.timeout)
	defer cancel()

	keys, err := c.scan(ctx, c.opts.prefix+"*", 1)
	if err != nil {
		panic(err)
	}
	if len(keys) == 0 {
		return zeroK, nil, false
	}

	splitKey := strings.SplitN(keys[0], c.opts.prefix, 2)
	if len(splitKey) != 2 {
		return zeroK, nil, false
	}

	var k K
	err = msgpack.Unmarshal([]byte(splitKey[1]), &k)
	if err != nil {
		panic(err)
	}
	v, err := c.redisClient.Get(ctx, keys[0]).Bytes()
	if err != nil {
		panic(err)
	}

	c.Delete(ctx, k)

	return k, v, true
}

func (c *mightyMapRedisStorage[K]) Range(ctx context.Context, f func(key K, value []byte) bool) {
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

		vb, err := c.redisClient.Get(ctx, key).Bytes()
		if err != nil {
			panic(err)
		}

		var k K
		err = msgpack.Unmarshal([]byte(splitKey[1]), &k)
		if err != nil {
			panic(err)
		}

		if !f(k, vb) {
			break
		}
	}
}

const defaultCursorSize int64 = 2048

func (c *mightyMapRedisStorage[K]) scan(ctx context.Context, keyPattern string, maxKeys ...int) ([]string, error) {
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
