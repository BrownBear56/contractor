package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/BrownBear56/contractor/internal/storage/memory"
	"go.uber.org/zap"
)

type FileStore struct {
	memoryStore memory.MemoryStore
	logger      logger.Logger
	filePath    string
}

func NewFileStore(filePath string, parentLogger logger.Logger) *FileStore {
	fs := &FileStore{
		memoryStore: *memory.NewMemoryStore(),
		filePath:    filePath,
		logger:      parentLogger,
	}
	_ = fs.loadFromFile()
	return fs
}

func (fs *FileStore) SaveID(id, originalURL string) error {
	if err := fs.memoryStore.SaveID(id, originalURL); err != nil {
		return fmt.Errorf("failed to save ID in memory store: %w", err)
	}
	if err := fs.saveToFile(); err != nil {
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

func (fs *FileStore) saveToFile() error {
	file, err := os.Create(fs.filePath)
	if err != nil {
		fs.logger.Error("Error creating file: %v\n", zap.Error(err))
		return fmt.Errorf("failed to create file %s: %w", fs.filePath, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fs.logger.Error("Error closing file: %v\n", zap.Error(err))
		}
	}()

	encoder := json.NewEncoder(file)
	for short, original := range fs.memoryStore.URLs {
		data := map[string]string{
			"short_url":    short,
			"original_url": original,
		}
		if err := encoder.Encode(data); err != nil {
			return fmt.Errorf("error encoding JSON: %w", err)
		}
	}
	return nil
}

func (fs *FileStore) loadFromFile() error {
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

		if err := fs.memoryStore.SaveID(data["short_url"], data["original_url"]); err != nil {
			return fmt.Errorf("failed to save ID in memory store: %w", err)
		}
	}
	return nil
}
