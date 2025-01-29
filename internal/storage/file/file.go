package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/BrownBear56/contractor/internal/storage/memory"
	"go.uber.org/zap"
)

type FileStore struct {
	mu          *sync.Mutex
	memoryStore memory.MemoryStore
	logger      logger.Logger
	filePath    string
}

func NewFileStore(filePath string, parentLogger logger.Logger) *FileStore {
	fs := &FileStore{
		mu:          &sync.Mutex{},
		memoryStore: *memory.NewMemoryStore(),
		filePath:    filePath,
		logger:      parentLogger,
	}
	_ = fs.loadFromFile()
	return fs
}

func (fs *FileStore) SaveID(userID, id, originalURL string) error {
	if err := fs.memoryStore.SaveID(userID, id, originalURL); err != nil {
		return fmt.Errorf("failed to save ID in memory store: %w", err)
	}
	if err := fs.appendToFile(userID, id, originalURL); err != nil {
		return fmt.Errorf("failed to save data to file: %w", err)
	}

	return nil
}

func (fs *FileStore) Get(id string) (string, bool) {
	return fs.memoryStore.Get(id)
}

func (fs *FileStore) GetIDByURL(originalURL string) (string, bool) {
	// Извлекаем ID по оригинальному URL из памяти
	return fs.memoryStore.GetIDByURL(originalURL)
}

func (fs *FileStore) GetUserURLs(userID string) (map[string]string, bool) {
	return fs.memoryStore.GetUserURLs(userID)
}

func (fs *FileStore) SaveBatch(userID string, pairs map[string]string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	for id, originalURL := range pairs {
		if err := fs.memoryStore.SaveID(userID, id, originalURL); err != nil {
			return fmt.Errorf("failed to save ID in memory store: %w", err)
		}
	}

	if err := fs.appendBatchToFile(pairs); err != nil {
		return fmt.Errorf("failed to save batch to file: %w", err)
	}

	return nil
}

func (fs *FileStore) appendToFile(userID, id, originalURL string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Открываем файл в режиме добавления, если его нет, создаем.
	const permLvl = 0o600
	file, err := os.OpenFile(fs.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, permLvl)
	if err != nil {
		fs.logger.Error("Error opening file: %v\n", zap.Error(err))
		return fmt.Errorf("failed to open file %s: %w", fs.filePath, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fs.logger.Error("Error closing file: %v\n", zap.Error(err))
		}
	}()

	// Подготавливаем данные для записи.
	data := map[string]string{
		"user_id":      userID,
		"short_url":    id,
		"original_url": originalURL,
	}

	// Кодируем данные в JSON и записываем их в файл.
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	return nil
}

func (fs *FileStore) loadFromFile() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	file, err := os.Open(fs.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("error loading from file: %w", err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			fs.logger.Error("Error closing file: %v\n", zap.Error(err))
		}
	}()

	decoder := json.NewDecoder(file)
	for {
		var data map[string]string
		if err := decoder.Decode(&data); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("error decoding JSON: %w", err)
		}

		if err := fs.memoryStore.SaveID(data["user_id"], data["short_url"], data["original_url"]); err != nil {
			return fmt.Errorf("failed to save ID in memory store: %w", err)
		}
	}
	return nil
}

func (fs *FileStore) appendBatchToFile(pairs map[string]string) error {
	const permLvl = 0o600
	file, err := os.OpenFile(fs.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, permLvl)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fs.logger.Error("Error closing file: %v\n", zap.Error(err))
		}
	}()

	encoder := json.NewEncoder(file)
	for id, originalURL := range pairs {
		data := map[string]string{
			"short_url":    id,
			"original_url": originalURL,
		}
		if err := encoder.Encode(data); err != nil {
			return fmt.Errorf("failed to encode data: %w", err)
		}
	}
	return nil
}
