package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	// SQLite driver - requires the following dependency:
	// go get github.com/mattn/go-sqlite3
	_ "github.com/mattn/go-sqlite3"
	msgpack "github.com/vmihailenco/msgpack/v5"
)

// mightyMapSQLiteStorage is the SQLite implementation of byteStorage interface
type mightyMapSQLiteStorage[K comparable] struct {
	db            *sql.DB
	mutex         *sync.RWMutex
	cachingMutex  *sync.RWMutex
	countCache    int
	lastCount     time.Time
	tableName     string
	cacheDuration time.Duration
}

type sqliteOpts struct {
	dbPath             string
	inMemory           bool
	pragmas            map[string]string
	tableName          string
	cacheCountDuration time.Duration
	maxOpenConns       int
	maxIdleConns       int
	journalMode        string
	syncMode           string
}

// Default options
const (
	defaultTableName          = "mightymap_kv"
	defaultCacheCountDuration = 5 * time.Second
	defaultMaxOpenConns       = 10
	defaultMaxIdleConns       = 5
	defaultJournalMode        = "WAL"
	defaultSyncMode           = "NORMAL"
)

// OptionFuncSQLite is a function type that modifies sqliteOpts configuration.
// It allows customizing the behavior of the SQLite storage implementation
// through functional options pattern.
type OptionFuncSQLite func(*sqliteOpts)

// NewMightyMapSQLiteStorage creates a new thread-safe storage implementation using SQLite.
// It accepts optional configuration through OptionFuncSQLite functions.
// Values are automatically encoded using MessagePack encoding.
//
// Parameters:
//   - optfuncs: Optional configuration functions that modify sqliteOpts settings
//
// Returns:
//   - IMightyMapStorage[K, V]: A new SQLite-backed storage implementation
//
// Panics if SQLite fails to open/initialize with the provided configuration.
func NewMightyMapSQLiteStorage[K comparable, V any](optfuncs ...OptionFuncSQLite) IMightyMapStorage[K, V] {
	// Default options
	opts := getDefaultSQLiteOptions()

	for _, optfunc := range optfuncs {
		optfunc(opts)
	}

	// Prepare connection string
	var dsn string
	if opts.inMemory {
		dsn = ":memory:"
	} else {
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(opts.dbPath), 0o755); err != nil {
			panic(fmt.Errorf("failed to create directory for SQLite database: %w", err))
		}
		dsn = opts.dbPath
	}

	// Add connection options
	dsn = fmt.Sprintf("%s?_journal_mode=%s&_synchronous=%s", dsn, opts.journalMode, opts.syncMode)

	// Open database connection
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		panic(fmt.Errorf("failed to open SQLite database: %w", err))
	}

	// Configure connection pool
	db.SetMaxOpenConns(opts.maxOpenConns)
	db.SetMaxIdleConns(opts.maxIdleConns)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		panic(fmt.Errorf("failed to connect to SQLite database: %w", err))
	}

	// Apply PRAGMA settings
	for pragma, value := range opts.pragmas {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA %s = %s", pragma, value)); err != nil {
			db.Close()
			panic(fmt.Errorf("failed to set PRAGMA %s: %w", pragma, err))
		}
	}

	// Create table if not exists
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			key BLOB PRIMARY KEY,
			value BLOB
		)`, opts.tableName)

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		panic(fmt.Errorf("failed to create table: %w", err))
	}

	// Create index on key for faster lookups
	createIndexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_key ON %s(key)
	`, opts.tableName, opts.tableName)

	if _, err := db.Exec(createIndexSQL); err != nil {
		db.Close()
		panic(fmt.Errorf("failed to create index: %w", err))
	}

	storage := &mightyMapSQLiteStorage[K]{
		db:            db,
		mutex:         &sync.RWMutex{},
		cachingMutex:  &sync.RWMutex{},
		countCache:    -1,
		lastCount:     time.Time{},
		tableName:     opts.tableName,
		cacheDuration: opts.cacheCountDuration,
	}

	return newMsgpackAdapter[K, V](storage)
}

// Load retrieves a value from the SQLite storage.
func (s *mightyMapSQLiteStorage[K]) Load(_ context.Context, key K) (value []byte, ok bool) {
	// Marshal the key to a byte slice
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		return nil, false
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Query the database
	var valueBytes []byte
	query := fmt.Sprintf("SELECT value FROM %s WHERE key = ?", s.getTableName())
	err = s.db.QueryRow(query, keyBytes).Scan(&valueBytes)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false
		}
		// Log the error but don't return it to maintain interface compatibility
		fmt.Printf("Error loading from SQLite: %v\n", err)
		return nil, false
	}

	return valueBytes, true
}

// Store adds or updates a key-value pair in the SQLite storage.
func (s *mightyMapSQLiteStorage[K]) Store(_ context.Context, key K, value []byte) {
	// Marshal the key to a byte slice
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Use INSERT OR REPLACE to handle both insert and update
	query := fmt.Sprintf("INSERT OR REPLACE INTO %s (key, value) VALUES (?, ?)", s.getTableName())
	_, err = s.db.Exec(query, keyBytes, value)
	if err != nil {
		// Log the error but don't return it to maintain interface compatibility
		fmt.Printf("Error storing to SQLite: %v\n", err)
	}

	// Invalidate count cache
	s.invalidateCountCache()
}

// Delete removes one or more keys from the SQLite storage.
func (s *mightyMapSQLiteStorage[K]) Delete(_ context.Context, keys ...K) {
	if len(keys) == 0 {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		fmt.Printf("Error starting transaction for delete: %v\n", err)
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	query := fmt.Sprintf("DELETE FROM %s WHERE key = ?", s.getTableName())
	stmt, err := tx.Prepare(query)
	if err != nil {
		fmt.Printf("Error preparing delete statement: %v\n", err)
		return
	}
	defer stmt.Close()

	for _, key := range keys {
		keyBytes, err := msgpack.Marshal(key)
		if err != nil {
			continue
		}

		_, err = stmt.Exec(keyBytes)
		if err != nil {
			fmt.Printf("Error deleting key from SQLite: %v\n", err)
		}
	}

	if err = tx.Commit(); err != nil {
		fmt.Printf("Error committing delete transaction: %v\n", err)
	}

	// Invalidate count cache
	s.invalidateCountCache()
}

// Range iterates over all key-value pairs in the SQLite storage.
func (s *mightyMapSQLiteStorage[K]) Range(_ context.Context, f func(key K, value []byte) bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	query := fmt.Sprintf("SELECT key, value FROM %s", s.getTableName())
	rows, err := s.db.Query(query)
	if err != nil {
		fmt.Printf("Error querying SQLite for range: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var keyBytes, valueBytes []byte
		if err := rows.Scan(&keyBytes, &valueBytes); err != nil {
			fmt.Printf("Error scanning row in range: %v\n", err)
			continue
		}

		var key K
		if err := msgpack.Unmarshal(keyBytes, &key); err != nil {
			fmt.Printf("Error unmarshalling key in range: %v\n", err)
			continue
		}

		if !f(key, valueBytes) {
			break
		}
	}

	if err := rows.Err(); err != nil {
		fmt.Printf("Error iterating rows in range: %v\n", err)
	}
}

func (s *mightyMapSQLiteStorage[K]) Keys(_ context.Context) []K {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	query := fmt.Sprintf("SELECT key FROM %s", s.getTableName())
	rows, err := s.db.Query(query)
	if err != nil {
		fmt.Printf("Error querying SQLite for keys: %v\n", err)
		return []K{}
	}
	defer rows.Close()

	keys := []K{}
	for rows.Next() {
		var keyBytes []byte
		if err := rows.Scan(&keyBytes); err != nil {
			fmt.Printf("Error scanning row in keys: %v\n", err)
			continue
		}

		var key K
		if err := msgpack.Unmarshal(keyBytes, &key); err != nil {
			fmt.Printf("Error unmarshalling key in keys: %v\n", err)
			continue
		}
		keys = append(keys, key)
	}

	return keys
}

// Next retrieves and removes the next key-value pair from the SQLite storage.
func (s *mightyMapSQLiteStorage[K]) Next(ctx context.Context) (key K, value []byte, ok bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	query := fmt.Sprintf("SELECT key, value FROM %s LIMIT 1", s.getTableName())
	row := s.db.QueryRow(query)

	var keyBytes []byte
	err := row.Scan(&keyBytes, &value)
	if err != nil {
		if err != sql.ErrNoRows {
			fmt.Printf("Error fetching next item: %v\n", err)
		}
		return key, nil, false
	}

	if err := msgpack.Unmarshal(keyBytes, &key); err != nil {
		fmt.Printf("Error unmarshalling key in next: %v\n", err)
		return key, nil, false
	}

	// Delete the retrieved key
	deleteQuery := fmt.Sprintf("DELETE FROM %s WHERE key = ?", s.getTableName())
	_, err = s.db.Exec(deleteQuery, keyBytes)
	if err != nil {
		fmt.Printf("Error deleting next item: %v\n", err)
	}

	// Invalidate count cache
	s.invalidateCountCache()

	return key, value, true
}

// Len returns the number of items in the SQLite storage.
func (s *mightyMapSQLiteStorage[K]) Len(_ context.Context) int {
	s.cachingMutex.RLock()
	if !s.lastCount.IsZero() && time.Since(s.lastCount) < s.getCacheCountDuration() {
		count := s.countCache
		s.cachingMutex.RUnlock()
		return count
	}
	s.cachingMutex.RUnlock()

	s.cachingMutex.Lock()
	defer s.cachingMutex.Unlock()

	// Check again after getting write lock to avoid race conditions
	if !s.lastCount.IsZero() && time.Since(s.lastCount) < s.getCacheCountDuration() {
		return s.countCache
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", s.getTableName())
	err := s.db.QueryRow(query).Scan(&count)
	if err != nil {
		fmt.Printf("Error counting items: %v\n", err)
		return 0
	}

	// Update cache
	s.countCache = count
	s.lastCount = time.Now()

	return count
}

// Clear removes all items from the SQLite storage.
func (s *mightyMapSQLiteStorage[K]) Clear(_ context.Context) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	query := fmt.Sprintf("DELETE FROM %s", s.getTableName())
	_, err := s.db.Exec(query)
	if err != nil {
		fmt.Printf("Error clearing SQLite storage: %v\n", err)
	}

	// Invalidate count cache
	s.invalidateCountCache()
}

// Close closes the SQLite database connection.
func (s *mightyMapSQLiteStorage[K]) Close(_ context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Helper methods

func (s *mightyMapSQLiteStorage[K]) getTableName() string {
	return s.tableName
}

func (s *mightyMapSQLiteStorage[K]) getCacheCountDuration() time.Duration {
	return s.cacheDuration
}

func (s *mightyMapSQLiteStorage[K]) invalidateCountCache() {
	s.cachingMutex.Lock()
	defer s.cachingMutex.Unlock()
	s.countCache = -1
	s.lastCount = time.Time{}
}

// Option functions

// WithSQLiteDBPath specifies the file path for the SQLite database.
// If not specified, an in-memory database will be used.
func WithSQLiteDBPath(path string) OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.dbPath = path
		o.inMemory = false
	}
}

// WithSQLiteInMemory specifies that an in-memory database should be used.
// This is faster but data will be lost when the application exits.
func WithSQLiteInMemory() OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.inMemory = true
	}
}

// WithSQLiteTableName sets a custom table name for the key-value store.
func WithSQLiteTableName(tableName string) OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.tableName = tableName
	}
}

// WithSQLiteJournalMode sets the journal mode for the SQLite database.
// Common values: DELETE, TRUNCATE, PERSIST, MEMORY, WAL, OFF
func WithSQLiteJournalMode(mode string) OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.journalMode = mode
	}
}

// WithSQLiteSyncMode sets the synchronous mode for the SQLite database.
// Common values: OFF, NORMAL, FULL, EXTRA
func WithSQLiteSyncMode(mode string) OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.syncMode = mode
	}
}

// WithSQLiteCountCacheDuration sets the duration for which the count result is cached.
func WithSQLiteCountCacheDuration(duration time.Duration) OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.cacheCountDuration = duration
	}
}

// WithSQLiteMaxOpenConns sets the maximum number of open connections to the database.
func WithSQLiteMaxOpenConns(count int) OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.maxOpenConns = count
	}
}

// WithSQLiteMaxIdleConns sets the maximum number of idle connections in the connection pool.
func WithSQLiteMaxIdleConns(count int) OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.maxIdleConns = count
	}
}

// WithSQLitePragma sets a custom PRAGMA option for the SQLite database.
func WithSQLitePragma(pragma, value string) OptionFuncSQLite {
	return func(o *sqliteOpts) {
		o.pragmas[pragma] = value
	}
}

// getDefaultSQLiteOptions returns the default configuration for SQLite storage.
func getDefaultSQLiteOptions() *sqliteOpts {
	return &sqliteOpts{
		dbPath:             "",
		inMemory:           true,
		pragmas:            make(map[string]string),
		tableName:          defaultTableName,
		cacheCountDuration: defaultCacheCountDuration,
		maxOpenConns:       defaultMaxOpenConns,
		maxIdleConns:       defaultMaxIdleConns,
		journalMode:        defaultJournalMode,
		syncMode:           defaultSyncMode,
	}
}

// Example usage:
//
//	// Create an in-memory SQLite storage
//	store := storage.NewMightyMapSQLiteStorage[string, int]()
//	mm := mightymap.New[string, int](true, store)
//	defer mm.Close(context.Background())
//
//	// Create a file-based SQLite storage with custom options
//	store := storage.NewMightyMapSQLiteStorage[string, User](
//		storage.WithSQLiteDBPath("/path/to/data.db"),
//		storage.WithSQLiteJournalMode("WAL"),
//		storage.WithSQLiteSyncMode("NORMAL"),
//		storage.WithSQLiteMaxOpenConns(20),
//		storage.WithSQLiteCountCacheDuration(10 * time.Second),
//		storage.WithSQLitePragma("cache_size", "-1000"), // 1MB cache
//	)
//	mm := mightymap.New[string, User](true, store)
//	defer mm.Close(context.Background())
