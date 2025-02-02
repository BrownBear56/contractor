package memory

import (
	"fmt"
	"sync"
)

type MemoryStore struct {
	mu          *sync.Mutex
	URLs        map[string]string
	reverseURLs map[string]string
	userURLs    map[string]map[string]string // user_id -> (short_id -> original_url)
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		mu:          &sync.Mutex{},
		URLs:        make(map[string]string),
		reverseURLs: make(map[string]string),
		userURLs:    make(map[string]map[string]string),
	}
}

func (s *MemoryStore) BatchDelete(userID string, urlIDs []string) error {
	return nil
}

func (s *MemoryStore) SaveID(userID, id, originalURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.URLs[id]; ok {
		return fmt.Errorf("ID %s already exists", id)
	}

	s.URLs[id] = originalURL
	s.reverseURLs[originalURL] = id

	if _, ok := s.userURLs[userID]; !ok {
		s.userURLs[userID] = make(map[string]string)
	}
	s.userURLs[userID][id] = originalURL
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

func (s *MemoryStore) GetUserURLs(userID string) (map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if urls, ok := s.userURLs[userID]; ok {
		return urls, true
	}
	return nil, false
}

func (s *MemoryStore) SaveBatch(userID string, pairs map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Инициализируем мапу для userID, если ее еще нет
	if _, ok := s.userURLs[userID]; !ok {
		s.userURLs[userID] = make(map[string]string)
	}

	for id, originalURL := range pairs {
		if _, ok := s.URLs[id]; ok {
			return fmt.Errorf("ID %s already exists", id)
		}
		if _, ok := s.userURLs[userID][id]; ok {
			return fmt.Errorf("ID %s already exists for user %s", id, userID)
		}

		s.URLs[id] = originalURL
		s.reverseURLs[originalURL] = id
		s.userURLs[userID][id] = originalURL
	}
	return nil
}
