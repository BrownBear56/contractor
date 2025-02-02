package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type PostgresStore struct {
	conn   *pgxpool.Pool
	logger logger.Logger
}

func NewPostgresStore(dsn string, parentLogger logger.Logger) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database DSN: %w", err)
	}

	pool, err := pgxpool.New(context.Background(), config.ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	pool.Config().MaxConns = 10

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
		id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		short_id VARCHAR(12) UNIQUE NOT NULL,
		original_url VARCHAR(255) UNIQUE NOT NULL,
		user_id VARCHAR(36) NOT NULL,
		is_deleted BOOLEAN DEFAULT FALSE
	);
	`
	if _, err := p.conn.Exec(context.Background(), query); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	return nil
}

// makePlaceholders создаёт плейсхолдеры для запроса ($2, $3, ..., $N).
func makePlaceholders(count, start int) []string {
	placeholders := make([]string, count)
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", start+i)
	}
	return placeholders
}

func (p *PostgresStore) BatchDelete(userID string, urlIDs []string) error {
	if len(urlIDs) == 0 {
		return nil
	}

	argumentCount := 2

	query := fmt.Sprintf(
		"UPDATE urls SET is_deleted = TRUE WHERE user_id = $1 AND short_id IN (%s)",
		strings.Join(makePlaceholders(len(urlIDs), argumentCount), ","),
	)

	args := make([]interface{}, len(urlIDs)+1)
	args[0] = userID
	for i, id := range urlIDs {
		args[i+1] = id
	}

	_, err := p.conn.Exec(context.Background(), query, args...)
	if err != nil {
		if wrappedErr := errors.Unwrap(err); wrappedErr != nil {
			p.logger.Error("Не удалось выполнить удаление с внутренней ошибкой", zap.Error(wrappedErr))
		} else {
			p.logger.Error("Не удалось выполнить удаление", zap.Error(err))
		}
		return fmt.Errorf("ошибка при удалении: %w", err)
	}
	return nil
}

func (p *PostgresStore) DeleteUserURLs(userID string, shortIDs []string) error {
	if len(shortIDs) == 0 {
		return nil
	}

	ctx := context.Background()
	tx, err := p.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			p.logger.Error("Failed to rollback transaction", zap.Error(err))
		}
	}()

	batch := &pgx.Batch{}
	for _, shortID := range shortIDs {
		batch.Queue(
			`UPDATE urls SET is_deleted = TRUE WHERE short_id = $1 AND user_id = $2`,
			shortID, userID,
		)
	}

	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		p.logger.Error("SendBatch error", zap.Error(err))
		return fmt.Errorf("send batch error: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		p.logger.Error("Failed to commit transaction", zap.Error(err))
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (p *PostgresStore) SaveID(userID, id, originalURL string) error {
	query := `INSERT INTO urls (short_id, original_url, user_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;`
	_, err := p.conn.Exec(context.Background(), query, id, originalURL, userID)
	if err != nil {
		// Здесь можно проверить, если ошибка обернута, и распаковать ее
		if wrappedErr := errors.Unwrap(err); wrappedErr != nil {
			p.logger.Error("Не удалось сохранить ID с внутренней ошибкой", zap.Error(wrappedErr))
		} else {
			p.logger.Error("Не удалось сохранить ID", zap.Error(err))
		}
		return fmt.Errorf("ошибка при сохранении ID: %w", err)
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

func (p *PostgresStore) GetUserURLs(userID string) (map[string]string, bool) {
	query := `SELECT short_id, original_url FROM urls WHERE user_id = $1;`
	rows, err := p.conn.Query(context.Background(), query, userID)
	if err != nil {
		p.logger.Error("Failed to get user URLs", zap.Error(err))
		return nil, false
	}
	defer rows.Close()

	urls := make(map[string]string)
	for rows.Next() {
		var shortID, originalURL string
		if err := rows.Scan(&shortID, &originalURL); err != nil {
			p.logger.Error("Failed to scan row", zap.Error(err))
			return nil, false
		}
		urls[shortID] = originalURL
	}
	return urls, true
}

func (p *PostgresStore) SaveBatch(userID string, pairs map[string]string) error {
	ctx := context.Background()
	tx, err := p.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			p.logger.Error("Failed to rollback transaction: %v\n", zap.Error(err))
		}
	}()

	batch := &pgx.Batch{}
	for id, originalURL := range pairs {
		batch.Queue(
			`INSERT INTO urls (short_id, original_url, user_id) 
			VALUES ($1, $2, $3) 
			ON CONFLICT DO NOTHING`,
			id, originalURL, userID,
		)
	}

	err = tx.SendBatch(ctx, batch).Close()
	if err != nil {
		p.logger.Error("SendBatch error: %v\n", zap.Error(err))
		return fmt.Errorf("send batch error: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		p.logger.Error("Failed to commit transaction", zap.Error(err))
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (p *PostgresStore) Close() {
	p.conn.Close()
}
