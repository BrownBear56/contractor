package memory

import (
	"fmt"
	"sync"
)

type MemoryStore struct {
	mu          *sync.Mutex
	URLs        map[string]string
	reverseURLs map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		mu:          &sync.Mutex{},
		URLs:        make(map[string]string),
		reverseURLs: make(map[string]string),
	}
}

func (s *MemoryStore) SaveID(id, originalURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.URLs[id]; ok {
		return fmt.Errorf("ID %s already exists", id)
	}

	s.URLs[id] = originalURL
	s.reverseURLs[originalURL] = id
	return nil
}

func (s *MemoryStore) Get(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	originalURL, ok := s.URLs[id]
	return originalURL, ok
}

func (s *MemoryStore) GetIDByURL(originalURL string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.reverseURLs[originalURL]
	return id, ok
}

func (s *MemoryStore) SaveBatch(pairs map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, originalURL := range pairs {
		if _, ok := s.URLs[id]; ok {
			return fmt.Errorf("ID %s already exists", id)
		}
		s.URLs[id] = originalURL
		s.reverseURLs[originalURL] = id
	}
	return nil
}
