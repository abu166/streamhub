package db

import (
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"rsshub/internal/config"
	"rsshub/internal/models"
)

type DB struct {
	*sql.DB
}

func NewDB(cfg *config.Config) (*DB, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.PGUser, cfg.PGPassword, cfg.PGHost, cfg.PGPort, cfg.PGDBName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	// Initialize schema
	err = initSchema(db)
	if err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

func initSchema(db *sql.DB) error {
	queries := []string{
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`,
		`CREATE TABLE IF NOT EXISTS feeds (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP,
			name TEXT UNIQUE NOT NULL,
			url TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS articles (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP,
			title TEXT NOT NULL,
			link TEXT NOT NULL,
			published_at TIMESTAMP NOT NULL,
			description TEXT,
			feed_id UUID REFERENCES feeds(id) ON DELETE CASCADE
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS articles_feed_link_idx ON articles (feed_id, link);`,
	}

	for _, q := range queries {
		_, err := db.Exec(q)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) AddFeed(feed *models.Feed) error {
	_, err := d.Exec(`INSERT INTO feeds (name, url) VALUES ($1, $2)`, feed.Name, feed.URL)
	return err
}

func (d *DB) ListFeeds(limit int) ([]models.Feed, error) {
	query := `SELECT id, created_at, updated_at, name, url FROM feeds ORDER BY created_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := d.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []models.Feed
	for rows.Next() {
		var f models.Feed
		var updated sql.NullTime
		err := rows.Scan(&f.ID, &f.CreatedAt, &updated, &f.Name, &f.URL)
		if err != nil {
			return nil, err
		}
		if updated.Valid {
			f.UpdatedAt = updated.Time
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
}

func (d *DB) DeleteFeed(name string) error {
	_, err := d.Exec(`DELETE FROM feeds WHERE name = $1`, name)
	return err
}

func (d *DB) GetArticles(feedName string, limit int) ([]models.Article, error) {
	query := `SELECT a.id, a.created_at, a.updated_at, a.title, a.link, a.published_at, a.description, a.feed_id
	FROM articles a
	JOIN feeds f ON a.feed_id = f.id
	WHERE f.name = $1
	ORDER BY a.published_at DESC
	LIMIT $2`

	rows, err := d.Query(query, feedName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []models.Article
	for rows.Next() {
		var a models.Article
		var updated sql.NullTime
		err := rows.Scan(&a.ID, &a.CreatedAt, &updated, &a.Title, &a.Link, &a.PublishedAt, &a.Description, &a.FeedID)
		if err != nil {
			return nil, err
		}
		if updated.Valid {
			a.UpdatedAt = updated.Time
		}
		articles = append(articles, a)
	}
	return articles, nil
}

func (d *DB) GetOutdatedFeeds(limit int) ([]models.Feed, error) {
	query := `SELECT id, created_at, updated_at, name, url FROM feeds ORDER BY updated_at ASC NULLS FIRST LIMIT $1`

	rows, err := d.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []models.Feed
	for rows.Next() {
		var f models.Feed
		var updated sql.NullTime
		err := rows.Scan(&f.ID, &f.CreatedAt, &updated, &f.Name, &f.URL)
		if err != nil {
			return nil, err
		}
		if updated.Valid {
			f.UpdatedAt = updated.Time
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
}

func (d *DB) ArticleExists(feedID uuid.UUID, link string) (bool, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM articles WHERE feed_id = $1 AND link = $2`, feedID, link).Scan(&count)
	return count > 0, err
}

func (d *DB) InsertArticle(article *models.Article) error {
	_, err := d.Exec(`INSERT INTO articles (title, link, published_at, description, feed_id)
		VALUES ($1, $2, $3, $4, $5)`, article.Title, article.Link, article.PublishedAt, article.Description, article.FeedID)
	return err
}

func (d *DB) UpdateFeedUpdatedAt(id uuid.UUID) error {
	_, err := d.Exec(`UPDATE feeds SET updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id)
	return err
}
