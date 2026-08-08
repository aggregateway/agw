package agw

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

// PayloadStore persists intercepted request/response bodies for the session
// journal. The file-backed implementation is the default; a js/wasm build can
// use MemoryPayloads instead so nothing touches the filesystem.
type PayloadStore interface {
	Create(key string) (PayloadFile, error)
	WriteRequest(key string, body []byte) error
	Read(key string, tail int64) ([]byte, error)
	Remove(key string) error
	Close() error
}

// PayloadFile is an append-only payload blob opened for writing.
type PayloadFile interface {
	io.Writer
	Size() int64
	Close() error
}

type filePayloadStore struct {
	dir        string
	persistent bool
}

// FilePayloads returns a PayloadStore backed by a temporary directory.
func FilePayloads() PayloadStore {
	directory, _ := os.MkdirTemp("", "agw-sessions-")
	return &filePayloadStore{dir: directory}
}

// FilePayloadsAt returns a durable PayloadStore rooted at dir. The directory
// is created on demand and its contents survive process restarts.
func FilePayloadsAt(dir string) PayloadStore {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return FilePayloads()
	}
	return &filePayloadStore{dir: dir, persistent: true}
}

func (s *filePayloadStore) path(key string) string { return filepath.Join(s.dir, key) }

func (s *filePayloadStore) Create(key string) (PayloadFile, error) {
	file, err := os.OpenFile(s.path(key), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &filePayloadFile{File: file}, nil
}

func (s *filePayloadStore) WriteRequest(key string, body []byte) error {
	return os.WriteFile(s.path(key), body, 0600)
}

func (s *filePayloadStore) Read(key string, tail int64) ([]byte, error) {
	if key == "" {
		return nil, os.ErrNotExist
	}
	file, err := os.Open(s.path(key))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if tail > 0 {
		if info, err := file.Stat(); err == nil && info.Size() > tail {
			if _, err := file.Seek(-tail, io.SeekEnd); err != nil {
				return nil, err
			}
		}
	}
	return io.ReadAll(file)
}

func (s *filePayloadStore) Remove(key string) error {
	if key == "" {
		return nil
	}
	return os.Remove(s.path(key))
}

func (s *filePayloadStore) Close() error {
	if s.persistent {
		return nil
	}
	if s.dir == "" {
		return nil
	}
	dir := s.dir
	s.dir = ""
	return os.RemoveAll(dir)
}

type filePayloadFile struct {
	*os.File
}

func (f *filePayloadFile) Size() int64 {
	if f.File == nil {
		return 0
	}
	if info, err := f.File.Stat(); err == nil {
		return info.Size()
	}
	return 0
}

// MemoryPayloads returns an in-memory PayloadStore that never touches the
// filesystem; suitable for js/wasm and tests.
func MemoryPayloads() PayloadStore {
	return &memoryPayloadStore{buffers: make(map[string][]byte)}
}

type memoryPayloadStore struct {
	mu      sync.Mutex
	buffers map[string][]byte
}

func (s *memoryPayloadStore) Create(key string) (PayloadFile, error) {
	return &memoryPayloadFile{store: s, key: key}, nil
}

func (s *memoryPayloadStore) WriteRequest(key string, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffers[key] = append([]byte(nil), body...)
	return nil
}

func (s *memoryPayloadStore) Read(key string, tail int64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf, ok := s.buffers[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	if tail > 0 && int64(len(buf)) > tail {
		return append([]byte(nil), buf[int64(len(buf))-tail:]...), nil
	}
	return append([]byte(nil), buf...), nil
}

func (s *memoryPayloadStore) Remove(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.buffers, key)
	return nil
}

func (s *memoryPayloadStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffers = nil
	return nil
}

type memoryPayloadFile struct {
	store *memoryPayloadStore
	key   string
}

func (f *memoryPayloadFile) Write(data []byte) (int, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	f.store.buffers[f.key] = append(f.store.buffers[f.key], data...)
	return len(data), nil
}

func (f *memoryPayloadFile) Size() int64 {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	return int64(len(f.store.buffers[f.key]))
}

func (f *memoryPayloadFile) Close() error { return nil }
