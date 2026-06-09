package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// initDatabase 连接数据库，若不存在则自动创建并运行迁移
func initDatabase(cfg *Config, logger *slog.Logger) (*sqlx.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		// 数据库不存在时自动创建
		if strings.Contains(err.Error(), "does not exist") {
			logger.Info("database not found, creating...", "dbname", cfg.Database.DBName)
			if err := createDatabase(cfg); err != nil {
				return nil, fmt.Errorf("create database: %w", err)
			}
			logger.Info("database created", "dbname", cfg.Database.DBName)
			db, err = sqlx.Connect("pgx", dsn)
			if err != nil {
				return nil, fmt.Errorf("connect after create: %w", err)
			}
		} else {
			return nil, fmt.Errorf("connect database: %w", err)
		}
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// 自动运行迁移
	if err := runMigrations(db, logger); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	logger.Info("database connected and migrated")
	return db, nil
}

// createDatabase 连接到默认 postgres 库并创建目标数据库
func createDatabase(cfg *Config) error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.SSLMode,
	)
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	// Quote identifier to prevent injection; name comes from config, not user input
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE \"%s\"", strings.ReplaceAll(cfg.Database.DBName, "\"", "")))
	return err
}

// runMigrations 从 migrations/ 目录读取 SQL 文件并执行（幂等）
func runMigrations(db *sqlx.DB, logger *slog.Logger) error {
	// 创建迁移追踪表
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR PRIMARY KEY,
		applied_at TIMESTAMPTZ DEFAULT NOW()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Strings(files)

	for _, path := range files {
		name := filepath.Base(path)

		// 检查是否已执行
		var exists bool
		err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", name)
		if err != nil || exists {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		// 只执行 ---- DOWN 之前的部分（UP migration）
		sql := string(content)
		if idx := strings.Index(sql, "---- DOWN"); idx != -1 {
			sql = sql[:idx]
		}

		if _, err := db.Exec(sql); err != nil {
			return fmt.Errorf("exec migration %s: %w", name, err)
		}

		_, _ = db.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", name)
		logger.Info("migration applied", "file", name)
	}

	return nil
}
