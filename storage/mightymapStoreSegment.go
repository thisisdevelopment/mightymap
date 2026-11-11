package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	segmentDefaultSize = 256 << 20 // 256 MB
	segmentFilePattern = "mightymap-*.seg"
	segmentFileName    = "mightymap-%08d.seg"
	segmentDirMode     = 0o750
	segmentFileMode    = 0o600
)

type segmentOpts struct {
	dir            string
	segmentSize    int64
	valueCacheSize int64
}

// OptionFuncSegment is a function type that modifies segmentOpts configuration.
// It allows customizing the behavior of the segment storage through functional options.
type OptionFuncSegment func(*segmentOpts)

func getDefaultSegmentOptions() *segmentOpts {
	return &segmentOpts{segmentSize: segmentDefaultSize}
}

// WithSegmentDir sets the directory the segment files are written to. It is
// created when it does not exist. Use one directory per storage: segment files
// that are already in it are removed when the storage opens.
// **Default value**: a new directory in `os.TempDir()`
func WithSegmentDir(dir string) OptionFuncSegment {
	return func(o *segmentOpts) {
		o.dir = dir
	}
}

// WithSegmentSize sets the size in bytes at which a segment file is full and
// the next one is started. Disk space comes back per file, once no value in
// the file is live, so smaller files give space back sooner and larger files
// keep fewer files open. A size of 0 or less is ignored.
// **Default value**: `256 << 20` (256 MB)
func WithSegmentSize(segmentSize int64) OptionFuncSegment {
	return func(o *segmentOpts) {
		if segmentSize > 0 {
			o.segmentSize = segmentSize
		}
	}
}

// WithSegmentValueCacheSize keeps recently loaded values in memory in their
// decoded form, up to this many bytes (counted by their encoded size), so that
// reading a hot value again does not read the file and decode it once more.
// Loaded values must then be treated as read-only: the same value is handed to
// every caller.
// **Default value**: `0` (no cache)
func WithSegmentValueCacheSize(valueCacheSize int64) OptionFuncSegment {
	return func(o *segmentOpts) {
		o.valueCacheSize = valueCacheSize
	}
}

// NewMightyMapSegmentStorage creates a thread-safe storage that keeps only the
// keys in memory and the values in files on disk. It needs nothing but the
// standard library.
//
// Values are encoded with MessagePack and appended to a segment file; the
// index in memory knows for every key in which file its value is, where it
// starts and how long it is, so a Load is one read at a known position. A full
// segment file is followed by a new one, and a file is removed as soon as none
// of its values is live any more. Nothing is ever rewritten: there is no
// compaction.
//
// That fits data that is written once, read often and removed in bulk, such as
// a cache that loads a new version of its data and then drops the previous
// one: the files of the previous version go as a whole. Space of values that
// are deleted or replaced one by one only comes back when every other value in
// the same file is gone too.
//
// The values do not survive a restart: the index is in memory only, and
// segment files left in the directory by an earlier run are removed when the
// storage opens. Close removes the files of this run.
//
// Panics if the directory cannot be created, and on Store if a value cannot
// be written.
func NewMightyMapSegmentStorage[K comparable, V any](optfuncs ...OptionFuncSegment) IMightyMapStorage[K, V] {
	opts := getDefaultSegmentOptions()
	for _, optfunc := range optfuncs {
		optfunc(opts)
	}

	adapter := newMsgpackAdapter[K, V](newSegmentStorage[K](opts.dir, opts.segmentSize))
	adapter.cache = newValueCache[K, V](opts.valueCacheSize)
	adapter.storageKeepsNothing = true // Store writes the value to a file and is done with it
	return adapter
}

type segmentStorage[K comparable] struct {
	// ponytail: one lock, also held while a value is written; a lock per
	// segment if writers ever queue up behind it. Reads only hold it for the
	// lookup in the index, not while they read from disk.
	mutex       sync.RWMutex
	dir         string
	segmentSize int64
	index       map[K]segmentEntry
	segments    map[*segment]struct{}
	current     *segment // the file that is written to; nil until the first Store
	nextID      uint64
}

// segmentEntry is where the value of one key is.
type segmentEntry struct {
	segment *segment
	offset  int64
	size    int
}

type segment struct {
	file *os.File
	size int64 // bytes written so far
	live int   // values in this file that are in the index
}

func newSegmentStorage[K comparable](dir string, segmentSize int64) *segmentStorage[K] {
	var err error
	if dir == "" {
		dir, err = os.MkdirTemp("", "mightymap-segments-")
	} else {
		err = os.MkdirAll(dir, segmentDirMode)
	}
	if err != nil {
		panic(fmt.Errorf("segment storage: %w", err))
	}

	// Without the index of the run that wrote them these files are of no use.
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if ok, _ := filepath.Match(segmentFilePattern, entry.Name()); ok && !entry.IsDir() {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}

	return &segmentStorage[K]{
		dir:         dir,
		segmentSize: segmentSize,
		index:       make(map[K]segmentEntry),
		segments:    make(map[*segment]struct{}),
	}
}

// Store appends the value to the current segment file and points the key at it.
// It must not keep value: the adapter reuses that memory for the next value.
func (s *segmentStorage[K]) Store(_ context.Context, key K, value []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.current == nil || s.current.size >= s.segmentSize {
		s.startSegment()
	}
	current := s.current
	// Written at the offset we keep ourselves: a write that fails halfway
	// leaves nothing behind that the next one does not overwrite.
	if _, err := current.file.WriteAt(value, current.size); err != nil {
		panic(fmt.Errorf("segment storage: storing value: %w", err))
	}

	s.forget(key)
	s.index[key] = segmentEntry{segment: current, offset: current.size, size: len(value)}
	current.size += int64(len(value))
	current.live++
}

// Load reads the value of key from its segment file.
func (s *segmentStorage[K]) Load(_ context.Context, key K) (value []byte, ok bool) {
	s.mutex.RLock()
	entry, ok := s.index[key]
	s.mutex.RUnlock()
	if !ok {
		return nil, false
	}

	// The file may be removed by now, when the value was deleted in the
	// meantime: the read then fails and the value is, rightly, not found.
	value, err := entry.read()
	return value, err == nil
}

// Delete removes the keys from the index, and every segment file that is left
// without live values.
func (s *segmentStorage[K]) Delete(_ context.Context, keys ...K) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, key := range keys {
		s.forget(key)
	}
}

// Range calls f for the keys that are stored when it starts, with the value
// each of them has when Range gets to it. The storage is not locked while f
// runs, so f may use it.
func (s *segmentStorage[K]) Range(ctx context.Context, f func(key K, value []byte) bool) {
	for _, key := range s.Keys(ctx) {
		value, ok := s.Load(ctx, key)
		if !ok {
			continue
		}
		if !f(key, value) {
			return
		}
	}
}

// Keys returns all keys, straight from the index.
func (s *segmentStorage[K]) Keys(_ context.Context) []K {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	keys := make([]K, 0, len(s.index))
	for key := range s.index {
		keys = append(keys, key)
	}
	return keys
}

// Next returns one key with its value and removes it from the storage.
func (s *segmentStorage[K]) Next(_ context.Context) (key K, value []byte, ok bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for candidate, entry := range s.index {
		candidateValue, err := entry.read()
		s.forget(candidate)
		if err != nil {
			continue // unreadable: gone either way, try the next one
		}
		return candidate, candidateValue, true
	}
	return key, nil, false
}

// Len returns the number of keys.
func (s *segmentStorage[K]) Len(_ context.Context) int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return len(s.index)
}

// Clear removes all keys and all segment files.
func (s *segmentStorage[K]) Clear(_ context.Context) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.reset()
}

// Close removes the segment files, which cannot be opened again, and the
// directory when nothing else is in it.
func (s *segmentStorage[K]) Close(_ context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.reset()
	_ = os.Remove(s.dir) // fails when it is not empty, which is what we want
	return nil
}

// The methods below expect the caller to hold the lock.

func (s *segmentStorage[K]) startSegment() {
	name := filepath.Join(s.dir, fmt.Sprintf(segmentFileName, s.nextID))
	file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, segmentFileMode) //nolint:gosec // the directory is the caller's choice
	if err != nil {
		panic(fmt.Errorf("segment storage: %w", err))
	}

	full := s.current
	s.current = &segment{file: file}
	s.segments[s.current] = struct{}{}
	s.nextID++
	if full != nil {
		s.removeIfDead(full)
	}
}

// forget takes key out of the index.
func (s *segmentStorage[K]) forget(key K) {
	entry, ok := s.index[key]
	if !ok {
		return
	}
	delete(s.index, key)
	entry.segment.live--
	s.removeIfDead(entry.segment)
}

// removeIfDead removes a segment file that holds no live value. The current
// one stays, it is still written to.
func (s *segmentStorage[K]) removeIfDead(seg *segment) {
	if seg.live > 0 || seg == s.current {
		return
	}
	s.remove(seg)
}

func (s *segmentStorage[K]) remove(seg *segment) {
	_ = seg.file.Close()
	_ = os.Remove(seg.file.Name())
	delete(s.segments, seg)
}

func (s *segmentStorage[K]) reset() {
	for seg := range s.segments {
		s.remove(seg)
	}
	s.index = make(map[K]segmentEntry)
	s.current = nil
}

func (e segmentEntry) read() ([]byte, error) {
	value := make([]byte, e.size)
	_, err := e.segment.file.ReadAt(value, e.offset)
	return value, err
}
