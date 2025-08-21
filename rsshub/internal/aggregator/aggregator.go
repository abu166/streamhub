package aggregator

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"rsshub/internal/db"
	"rsshub/internal/models"
	"rsshub/internal/rss"
)

type Aggregator struct {
	db        *sql.DB
	interval  time.Duration
	workers   int
	sockPath  string
	ticker    *time.Ticker
	jobs      chan models.Feed
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	listener  net.Listener
	doneChans []chan struct{}
}

func NewAggregator(db *sql.DB, interval time.Duration, workers int, sockPath string) *Aggregator {
	return &Aggregator{
		db:        db,
		interval:  interval,
		workers:   workers,
		sockPath:  sockPath,
		doneChans: []chan struct{}{},
	}
}

func (a *Aggregator) Start(parentCtx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(parentCtx)
	a.ticker = time.NewTicker(a.interval)
	a.jobs = make(chan models.Feed, a.workers)

	for i := 0; i < a.workers; i++ {
		done := make(chan struct{})
		a.doneChans = append(a.doneChans, done)
		a.wg.Add(1)
		go a.worker(done)
	}

	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-a.ticker.C:
				database := &db.DB{DB: a.db}
				feeds, err := database.GetOutdatedFeeds(a.workers)
				if err != nil {
					fmt.Printf("Error getting outdated feeds: %v\n", err)
					continue
				}
				fmt.Printf("Ticker tick: Processing %d outdated feeds\n", len(feeds)) // Debug
				for _, feed := range feeds {
					a.jobs <- feed
				}
			}
		}
	}()

	var err error
	a.listener, err = net.Listen("unix", a.sockPath)
	if err != nil {
		return err
	}
	go a.controlLoop()

	return nil
}

func (a *Aggregator) Stop() error {
	a.cancel()
	a.ticker.Stop()
	close(a.jobs)
	for _, done := range a.doneChans {
		close(done)
	}
	a.wg.Wait()
	a.listener.Close()
	os.Remove(a.sockPath)
	return nil
}

func (a *Aggregator) worker(done chan struct{}) {
	defer a.wg.Done()
	database := &db.DB{DB: a.db}
	for {
		select {
		case feed := <-a.jobs:
			fmt.Printf("Worker fetching feed: %s (%s)\n", feed.Name, feed.URL) // Debug log
			rssFeed, err := rss.FetchAndParse(feed.URL)
			if err != nil {
				fmt.Printf("Error fetching/parsing feed %s: %v\n", feed.URL, err)
				continue
			}
			itemCount := len(rssFeed.Channel.Item)
			fmt.Printf("Parsed %d items from feed %s\n", itemCount, feed.Name) // Debug
			for _, item := range rssFeed.Channel.Item {
				pubDate, err := parsePubDate(item.PubDate)
				if err != nil {
					fmt.Printf("Error parsing pubDate '%s' for item %s: %v\n", item.PubDate, item.Link, err)
					continue
				}
				article := models.Article{
					Title:       item.Title,
					Link:        item.Link,
					Description: item.Description,
					PublishedAt: pubDate,
					FeedID:      feed.ID,
				}
				exists, err := database.ArticleExists(feed.ID, article.Link)
				if err != nil {
					fmt.Printf("Error checking if article exists: %v\n", err)
					continue
				}
				if exists {
					fmt.Printf("Article already exists: %s\n", article.Link) // Debug
					continue
				}
				err = database.InsertArticle(&article)
				if err != nil {
					fmt.Printf("Error inserting article %s: %v\n", article.Link, err)
				} else {
					fmt.Printf("Inserted article: %s\n", article.Title) // Debug
				}
			}
			err = database.UpdateFeedUpdatedAt(feed.ID)
			if err != nil {
				fmt.Printf("Error updating feed %s: %v\n", feed.URL, err)
			}
		case <-done:
			return
		case <-a.ctx.Done():
			return
		}
	}
}

// Helper for robust pubDate parsing
func parsePubDate(s string) (time.Time, error) {
	formats := []string{
		time.RFC1123,  // e.g., "Tue, 20 Aug 2024 10:20:30 GMT"
		time.RFC1123Z, // e.g., "Tue, 20 Aug 2024 10:20:30 -0000"
		time.RFC822,   // Similar, but with 2-digit year
		time.RFC822Z,
		"2006-01-02T15:04:05Z", // ISO 8601 variant
		"2006-01-02T15:04:05-07:00",
	}
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("no matching format for pubDate: %s", s)
}

func (a *Aggregator) Resize(newWorkers int) error {
	if newWorkers < 1 {
		return fmt.Errorf("workers must be at least 1")
	}
	oldWorkers := a.workers
	a.workers = newWorkers
	if newWorkers > oldWorkers {
		for i := oldWorkers; i < newWorkers; i++ {
			done := make(chan struct{})
			a.doneChans = append(a.doneChans, done)
			a.wg.Add(1)
			go a.worker(done)
		}
	} else if newWorkers < oldWorkers {
		for i := newWorkers; i < oldWorkers; i++ {
			close(a.doneChans[i])
		}
		a.doneChans = a.doneChans[:newWorkers]
	}
	fmt.Printf("Resized workers from %d to %d\n", oldWorkers, newWorkers) // Debug
	return nil
}

func (a *Aggregator) controlLoop() {
	for {
		conn, err := a.listener.Accept()
		if err != nil {
			continue // Allow graceful shutdown
		}
		go a.handleControl(conn)
	}
}

func (a *Aggregator) handleControl(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}
	cmd := strings.TrimSpace(string(buf[:n]))
	parts := strings.Split(cmd, " ")
	if len(parts) < 2 {
		return
	}
	switch parts[0] {
	case "set-interval":
		dur, err := time.ParseDuration(parts[1])
		if err != nil {
			conn.Write([]byte("Invalid duration\n"))
			return
		}
		old := a.interval
		a.interval = dur
		a.ticker.Reset(dur)
		conn.Write([]byte(fmt.Sprintf("Interval of fetching feeds changed from %s to %s\n", old, dur)))
	case "set-workers":
		count, err := strconv.Atoi(parts[1])
		if err != nil {
			conn.Write([]byte("Invalid count\n"))
			return
		}
		old := a.workers
		err = a.Resize(count)
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("Error resizing workers: %v\n", err)))
			return
		}
		conn.Write([]byte(fmt.Sprintf("Number of workers changed from %d to %d\n", old, count)))
	}
}
