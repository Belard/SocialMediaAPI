package database

import (
	"database/sql"

	_ "github.com/lib/pq"
)

type Database struct {
	DB *sql.DB
}

func NewDatabase(connStr string) (*Database, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	database := &Database{DB: db}
	if err := database.createTables(); err != nil {
		return nil, err
	}

	return database, nil
}

func (d *Database) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id VARCHAR(255) PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS media (
			id VARCHAR(255) PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			filename VARCHAR(255) NOT NULL,
			path VARCHAR(500) NOT NULL,
			url VARCHAR(500) NOT NULL,
			type VARCHAR(50) NOT NULL,
			size BIGINT NOT NULL,
			mime_type VARCHAR(100) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id VARCHAR(255) PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			content TEXT NOT NULL,
			post_type VARCHAR(50) NOT NULL DEFAULT 'normal',
			media_ids TEXT[],
			platforms TEXT[] NOT NULL,
			status VARCHAR(50) NOT NULL,
			scheduled_for TIMESTAMP,
			published_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		// Migration: add post_type column to existing tables
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='posts' AND column_name='post_type') THEN
				ALTER TABLE posts ADD COLUMN post_type VARCHAR(50) NOT NULL DEFAULT 'normal';
			END IF;
		END $$;`,
		// Migration: add is_sponsored column to existing tables
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='posts' AND column_name='is_sponsored') THEN
				ALTER TABLE posts ADD COLUMN is_sponsored BOOLEAN NOT NULL DEFAULT false;
			END IF;
		END $$;`,
		// Migration: add privacy_level column to existing tables
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='posts' AND column_name='privacy_level') THEN
				ALTER TABLE posts ADD COLUMN privacy_level VARCHAR(50) NOT NULL DEFAULT 'public';
			END IF;
		END $$;`,
		`CREATE TABLE IF NOT EXISTS credentials (
			id VARCHAR(255) PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			platform VARCHAR(50) NOT NULL,
			access_token TEXT NOT NULL,
			refresh_token TEXT,
			secret TEXT,
			token_type VARCHAR(50) DEFAULT 'Bearer',
			expires_at TIMESTAMP,
			platform_user_id VARCHAR(255),
			platform_page_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, platform),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS publish_results (
			id SERIAL PRIMARY KEY,
			post_id VARCHAR(255) NOT NULL,
			platform VARCHAR(50) NOT NULL,
			success BOOLEAN NOT NULL,
			message TEXT,
			external_post_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE
		)`,
	}

	for _, query := range queries {
		if _, err := d.DB.Exec(query); err != nil {
			return err
		}
	}

	return nil
}