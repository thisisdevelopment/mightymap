# MightyMap Migration Tools

These CLI tools help migrate data from MightyMap v0.4.5 (direct storage) to v0.5.1 (MessagePack encoding).

## Overview

- **migrate-badger**: Migrates BadgerDB storage from v0.4.5 to v0.5.1
- **migrate-redis**: Migrates Redis storage from v0.4.5 to v0.5.1

Both tools support:
- ✅ Dry-run mode to preview changes
- ✅ Configurable batch processing  
- ✅ Progress logging and statistics
- ✅ Error handling and reporting
- ✅ Key pattern filtering
- ✅ Timeout controls

## Setup

### Prerequisites

1. **Go 1.21+** installed
2. **Access to both versions** of MightyMap:
   - v0.4.5 for reading legacy data
   - v0.5.1 for writing new format

### Installation Steps

1. **Create a migration workspace:**
   ```bash
   mkdir mightymap-migration
   cd mightymap-migration
   ```

2. **Get current version (v0.5.1):**
   ```bash
   git clone https://github.com/thisisdevelopment/mightymap.git current
   cd current
   git checkout v0.5.1  # or latest
   ```

3. **Get legacy version (v0.4.5):**
   ```bash
   cd ..
   git clone https://github.com/thisisdevelopment/mightymap.git legacy
   cd legacy
   git checkout v0.4.5
   ```

4. **Copy migration tools:**
   ```bash
   cd ..
   cp -r current/cmd/migrate-* .
   
   # Your structure should look like:
   # mightymap-migration/
   # ├── current/          # v0.5.1 code
   # ├── legacy/           # v0.4.5 code  
   # ├── migrate-badger/   # Migration tool
   # └── migrate-redis/    # Migration tool
   ```

5. **Initialize migration modules:**
   ```bash
   # For Badger migration
   cd migrate-badger
   go mod tidy
   
   # For Redis migration  
   cd ../migrate-redis
   go mod tidy
   ```

   The go.mod files are already configured with the correct replace directives pointing to the local directories.

## Usage

### BadgerDB Migration

1. **Create configuration:**
   ```bash
   cd migrate-badger
   go run main.go  # Creates migrate-badger.yaml
   ```

2. **Edit configuration file (`migrate-badger.yaml`):**
   ```yaml
   source:
     dir: "./data/legacy-badger"
     memory_storage: false
     compression: false
   target:
     dir: "./data/migrated-badger"  
     memory_storage: false
     compression: true
   migration:
     batch_size: 1000
     log_interval: 100
     timeout: 30m
     key_pattern: "*"
   ```

3. **Run dry-run:**
   ```bash
   go run main.go --dry-run --verbose
   ```

4. **Run actual migration:**
   ```bash
   go run main.go --verbose
   ```

### Redis Migration

1. **Create configuration:**
   ```bash
   cd migrate-redis
   go run main.go  # Creates migrate-redis.yaml
   ```

2. **Edit configuration file (`migrate-redis.yaml`):**
   ```yaml
   source:
     addr: "localhost:6379"
     username: ""
     password: ""
     db: 0
     prefix: "mightymap_legacy_"
   target:
     addr: "localhost:6379" 
     username: ""
     password: ""
     db: 1  # Different DB
     prefix: "mightymap_v05_"
   migration:
     batch_size: 1000
     log_interval: 100
     timeout: 30m
     key_pattern: "*"
   ```

3. **Run dry-run:**
   ```bash
   go run main.go --dry-run --verbose
   ```

4. **Run actual migration:**
   ```bash
   go run main.go --verbose
   ```

## Configuration Options

### BadgerDB Config
- `dir`: Database directory path
- `memory_storage`: Use in-memory storage
- `compression`: Enable compression
- `encryption_key`: Encryption key (if used)

### Redis Config  
- `addr`: Redis server address
- `username`: Redis username (new in v0.5.1!)
- `password`: Redis password
- `db`: Database number
- `prefix`: Key prefix

### Migration Config
- `batch_size`: Keys processed before pause
- `log_interval`: Progress logging frequency
- `timeout`: Maximum migration time
- `key_pattern`: Filter keys to migrate
- `source_prefix`/`target_prefix`: Transform key prefixes

## Command Line Options

```bash
--config string     Configuration file path (default: "migrate-{tool}.yaml")
--dry-run          Show what would be migrated without making changes
--verbose          Enable detailed logging
```

## Example Output

```
🔍 DRY RUN MODE - No changes will be made
=====================================
📖 Reading from: ./data/legacy-badger
📝 Writing to: ./data/migrated-badger

🔄 Would migrate: user:123 → string
🔄 Would migrate: session:abc → map[string]interface{}
🔄 Would migrate: counter:1 → int
Processed 100 keys...

📊 Migration Statistics
=====================
Total Keys:     1543
Migrated:       1543
Skipped:        0
Errors:         0
Duration:       2.3s
Rate:           671.3 keys/sec

✨ Dry run completed. Use --dry-run=false to perform actual migration.
```

## Safety Features

- **Dry-run mode**: Preview all changes before execution
- **Separate target directories/databases**: No risk of overwriting source data
- **Batch processing**: Controlled memory usage and system load
- **Timeout protection**: Prevents runaway migrations
- **Error tracking**: Detailed error reporting
- **Progress logging**: Monitor migration status

## Troubleshooting

### Import Errors
- Ensure both v0.4.5 and v0.5.1 are properly checked out
- Verify go.mod replace directives point to correct paths
- Run `go mod tidy` after setup

### Memory Issues  
- Reduce `batch_size` in configuration
- Use `memory_storage: true` for BadgerDB if dealing with large datasets

### Connection Issues (Redis)
- Verify Redis server is running
- Check authentication credentials
- Ensure network connectivity

## Security Notes

- **Backup your data** before running migrations
- Use **separate target locations** to avoid data loss
- Review **dry-run output** carefully before proceeding
- Consider **authentication** requirements for Redis

## Support

If you encounter issues:
1. Run with `--dry-run --verbose` first
2. Check configuration file syntax
3. Verify both MightyMap versions are available
4. Review error messages in migration statistics

The migration preserves all data integrity while converting to the new MessagePack format for better type safety and performance. 