package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	// Legacy v0.4.5 import - using new module name
	legacymightymap "github.com/thisisdevelopment/mightymap-legacy"
	legacystorage "github.com/thisisdevelopment/mightymap-legacy/storage"

	// Current v0.5.0 import
	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

type Config struct {
	Source    BadgerConfig    `yaml:"source"`
	Target    BadgerConfig    `yaml:"target"`
	Migration MigrationConfig `yaml:"migration"`
}

type BadgerConfig struct {
	Dir                   string        `yaml:"dir"`
	MemoryStorage         bool          `yaml:"memory_storage"`
	Compression           bool          `yaml:"compression"`
	NumCompactors         int           `yaml:"num_compactors"`
	NumVersionsToKeep     int           `yaml:"num_versions_to_keep"`
	IndexCacheSize        int64         `yaml:"index_cache_size"`
	BlockCacheSize        int64         `yaml:"block_cache_size"`
	BlockSize             int           `yaml:"block_size"`
	MemTableSize          int64         `yaml:"mem_table_size"`
	ValueThreshold        int64         `yaml:"value_threshold"`
	SyncWrites            bool          `yaml:"sync_writes"`
	EncryptionKey         string        `yaml:"encryption_key"`
	EncryptionKeyRotation time.Duration `yaml:"encryption_key_rotation"`
}

type MigrationConfig struct {
	BatchSize      int           `yaml:"batch_size"`
	LogInterval    int           `yaml:"log_interval"`
	BackupOriginal bool          `yaml:"backup_original"`
	Timeout        time.Duration `yaml:"timeout"`
	KeyPattern     string        `yaml:"key_pattern"` // Optional filter
}

var (
	configFile = flag.String("config", "migrate-badger.yaml", "Configuration file path")
	dryRun     = flag.Bool("dry-run", false, "Show what would be migrated without making changes")
	verbose    = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()
	if config.Migration.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Migration.Timeout)
		defer cancel()
	}

	if *dryRun {
		fmt.Println("🔍 DRY RUN MODE - No changes will be made")
		fmt.Println("=====================================")
	}

	// Run migration
	stats, err := runMigration(ctx, config, *dryRun, *verbose)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Print results
	printStats(stats, *dryRun)
}

type MigrationStats struct {
	TotalKeys    int
	MigratedKeys int
	SkippedKeys  int
	ErrorKeys    int
	StartTime    time.Time
	EndTime      time.Time
	Errors       []string
}

func runMigration(ctx context.Context, config *Config, dryRun, verbose bool) (*MigrationStats, error) {
	stats := &MigrationStats{
		StartTime: time.Now(),
		Errors:    make([]string, 0),
	}

	// Create legacy storage (v0.4.5) for reading
	legacyStore := createLegacyBadgerStorage(config.Source)
	defer legacyStore.Close(ctx)

	// Create new storage (v0.5.0) for writing
	var newStore *mightymap.Map[string, interface{}]
	if !dryRun {
		newStore = createNewBadgerStorage(config.Target)
		defer newStore.Close(ctx)
	}

	if verbose {
		fmt.Printf("📖 Reading from: %s\n", config.Source.Dir)
		if !dryRun {
			fmt.Printf("📝 Writing to: %s\n", config.Target.Dir)
		}
		fmt.Println()
	}

	// Iterate through all keys in legacy storage
	batchCount := 0
	legacyStore.Range(ctx, func(key string, value interface{}) bool {
		stats.TotalKeys++

		if verbose && stats.TotalKeys%config.Migration.LogInterval == 0 {
			fmt.Printf("Processed %d keys...\n", stats.TotalKeys)
		}

		// Apply key pattern filter if specified
		if config.Migration.KeyPattern != "" {
			matched, err := matchPattern(key, config.Migration.KeyPattern)
			if err != nil {
				stats.ErrorKeys++
				stats.Errors = append(stats.Errors, fmt.Sprintf("Pattern match error for key %s: %v", key, err))
				return true
			}
			if !matched {
				stats.SkippedKeys++
				if verbose {
					fmt.Printf("⏭️  Skipped key (pattern): %s\n", key)
				}
				return true
			}
		}

		if dryRun {
			fmt.Printf("🔄 Would migrate: %s → %T\n", key, value)
			stats.MigratedKeys++
		} else {
			// Migrate the key-value pair
			err := migrateKeyValue(ctx, newStore, key, value)
			if err != nil {
				stats.ErrorKeys++
				stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to migrate key %s: %v", key, err))
				if verbose {
					fmt.Printf("❌ Error migrating key %s: %v\n", key, err)
				}
			} else {
				stats.MigratedKeys++
				if verbose {
					fmt.Printf("✅ Migrated: %s → %T\n", key, value)
				}
			}
		}

		batchCount++
		if batchCount >= config.Migration.BatchSize {
			// Small pause to prevent overwhelming the system
			time.Sleep(10 * time.Millisecond)
			batchCount = 0
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			fmt.Println("\n⚠️  Migration cancelled by timeout or signal")
			return false
		default:
			return true
		}
	})

	stats.EndTime = time.Now()

	return stats, nil
}

func createLegacyBadgerStorage(config BadgerConfig) *legacymightymap.Map[string, interface{}] {
	// Configure legacy storage with v0.4.5 API
	var opts []legacystorage.OptionFuncBadger

	if config.Dir != "" {
		opts = append(opts, legacystorage.WithTempDir(config.Dir))
	}
	opts = append(opts, legacystorage.WithMemoryStorage(config.MemoryStorage))
	opts = append(opts, legacystorage.WithCompression(config.Compression))

	if config.NumCompactors > 0 {
		opts = append(opts, legacystorage.WithNumCompactors(config.NumCompactors))
	}
	if config.EncryptionKey != "" {
		opts = append(opts, legacystorage.WithEncryptionKey(config.EncryptionKey))
	}

	store := legacystorage.NewMightyMapBadgerStorage[string, interface{}](opts...)
	return legacymightymap.New[string, interface{}](true, store)
}

func createNewBadgerStorage(config BadgerConfig) *mightymap.Map[string, interface{}] {
	// Configure new storage with v0.5.0 API
	var opts []storage.OptionFuncBadger

	if config.Dir != "" {
		opts = append(opts, storage.WithTempDir(config.Dir))
	}
	opts = append(opts, storage.WithMemoryStorage(config.MemoryStorage))
	opts = append(opts, storage.WithCompression(config.Compression))

	if config.NumCompactors > 0 {
		opts = append(opts, storage.WithNumCompactors(config.NumCompactors))
	}
	if config.EncryptionKey != "" {
		opts = append(opts, storage.WithEncryptionKey(config.EncryptionKey))
	}

	store := storage.NewMightyMapBadgerStorage[string, interface{}](opts...)
	return mightymap.New[string, interface{}](true, store)
}

func migrateKeyValue(ctx context.Context, newStore *mightymap.Map[string, interface{}], key string, value interface{}) error {
	// Simply store using new format - MessagePack will handle serialization
	newStore.Store(ctx, key, value)
	return nil
}

func matchPattern(key, pattern string) (bool, error) {
	// Simple pattern matching - could be enhanced with regex
	if pattern == "*" || pattern == "" {
		return true, nil
	}
	// For now, simple string contains
	return key == pattern, nil
}

func loadConfig(path string) (*Config, error) {
	// Create default config if file doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		defaultConfig := createDefaultConfig()
		if err := saveConfig(defaultConfig, path); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		fmt.Printf("📄 Created default config file: %s\n", path)
		fmt.Println("Please review and modify the configuration before running again.")
		os.Exit(0)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func createDefaultConfig() *Config {
	return &Config{
		Source: BadgerConfig{
			Dir:           "./data/badger-legacy",
			MemoryStorage: false,
			Compression:   false,
		},
		Target: BadgerConfig{
			Dir:           "./data/badger-migrated",
			MemoryStorage: false,
			Compression:   true, // Enable compression for new format
		},
		Migration: MigrationConfig{
			BatchSize:      1000,
			LogInterval:    100,
			BackupOriginal: true,
			Timeout:        30 * time.Minute,
			KeyPattern:     "*", // Migrate all keys
		},
	}
}

func saveConfig(config *Config, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printStats(stats *MigrationStats, dryRun bool) {
	duration := stats.EndTime.Sub(stats.StartTime)

	fmt.Println()
	fmt.Println("📊 Migration Statistics")
	fmt.Println("=====================")
	fmt.Printf("Total Keys:     %d\n", stats.TotalKeys)
	fmt.Printf("Migrated:       %d\n", stats.MigratedKeys)
	fmt.Printf("Skipped:        %d\n", stats.SkippedKeys)
	fmt.Printf("Errors:         %d\n", stats.ErrorKeys)
	fmt.Printf("Duration:       %v\n", duration)

	if stats.TotalKeys > 0 {
		rate := float64(stats.TotalKeys) / duration.Seconds()
		fmt.Printf("Rate:           %.1f keys/sec\n", rate)
	}

	if len(stats.Errors) > 0 {
		fmt.Println("\n❌ Errors:")
		for i, err := range stats.Errors {
			fmt.Printf("  %d. %s\n", i+1, err)
			if i >= 9 { // Limit error display
				fmt.Printf("  ... and %d more errors\n", len(stats.Errors)-10)
				break
			}
		}
	}

	if dryRun {
		fmt.Println("\n✨ Dry run completed. Use --dry-run=false to perform actual migration.")
	} else {
		fmt.Println("\n✅ Migration completed successfully!")
	}
}
