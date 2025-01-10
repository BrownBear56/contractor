package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/jackc/pgx/v4/pgxpool"
	"go.uber.org/zap"
)

type PostgresStore struct {
	conn   *pgxpool.Pool
	logger logger.Logger
}

func NewPostgresStore(dsn string, parentLogger logger.Logger) (*PostgresStore, error) {
	pool, err := pgxpool.Connect(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	store := &PostgresStore{
		conn:   pool,
		logger: parentLogger,
	}

	if err := store.createSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

func (p *PostgresStore) createSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS urls (
		id SERIAL PRIMARY KEY,
		short_id TEXT UNIQUE NOT NULL,
		original_url TEXT NOT NULL
	);
	`
	if _, err := p.conn.Exec(context.Background(), query); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	return nil
}

func (p *PostgresStore) SaveID(id, originalURL string) error {
	query := `INSERT INTO urls (short_id, original_url) VALUES ($1, $2) ON CONFLICT DO NOTHING;`
	_, err := p.conn.Exec(context.Background(), query, id, originalURL)
	if err != nil {
		// Здесь можно проверить, если ошибка обернута, и распаковать ее
		if wrappedErr := errors.Unwrap(err); wrappedErr != nil {
			p.logger.Error("Не удалось сохранить ID с внутренней ошибкой", zap.Error(wrappedErr))
		} else {
			p.logger.Error("Не удалось сохранить ID", zap.Error(err))
		}
		return fmt.Errorf("ошибка при сохранении ID: %w", err) // Обернуть ошибку правильно
	}
	return nil
}

func (p *PostgresStore) Get(id string) (string, bool) {
	query := `SELECT original_url FROM urls WHERE short_id = $1;`
	var originalURL string
	err := p.conn.QueryRow(context.Background(), query, id).Scan(&originalURL)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", false
		}
		p.logger.Error("Failed to get URL", zap.Error(err))
		return "", false
	}
	return originalURL, true
}

func (p *PostgresStore) GetIDByURL(originalURL string) (string, bool) {
	query := `SELECT short_id FROM urls WHERE original_url = $1;`
	var id string
	err := p.conn.QueryRow(context.Background(), query, originalURL).Scan(&id)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", false
		}
		p.logger.Error("Failed to get ID", zap.Error(err))
		return "", false
	}
	return id, true
}

func (p *PostgresStore) Close() {
	p.conn.Close()
}
