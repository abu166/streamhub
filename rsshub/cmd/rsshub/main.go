package main

import (
	"context"
	"flag"
	"fmt"
	_ "github.com/lib/pq"
	"net"
	"os"
	"os/signal"
	"rsshub/internal/aggregator"
	"rsshub/internal/config"
	"rsshub/internal/db"
	"rsshub/internal/models"
	"syscall"
)

const sockPath = "/tmp/rsshub.sock"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	command := os.Args[1]

	cfg := config.LoadConfig()

	database, err := db.NewDB(cfg)
	if err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	switch command {
	case "fetch":
		handleFetch(cfg, database)
	case "add":
		handleAdd(database)
	case "list":
		handleList(database)
	case "delete":
		handleDelete(database)
	case "articles":
		handleArticles(database)
	case "set-interval":
		handleSetInterval()
	case "set-workers":
		handleSetWorkers()
	case "--help":
		printHelp()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

func handleFetch(cfg *config.Config, database *db.DB) {
	// Check if already running
	_, err := net.Dial("unix", sockPath)
	if err == nil {
		fmt.Println("Background process is already running")
		return
	}
	// Clean up stale socket if exists
	os.Remove(sockPath)

	agg := aggregator.NewAggregator(database.DB, cfg.Interval, cfg.Workers, sockPath)

	err = agg.Start(context.Background())
	if err != nil {
		fmt.Printf("Error starting aggregator: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("The background process for fetching feeds has started (interval = %s, workers = %d)\n", cfg.Interval, cfg.Workers)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	err = agg.Stop()
	if err != nil {
		fmt.Printf("Error stopping aggregator: %v\n", err)
	}
	fmt.Println("Graceful shutdown: aggregator stopped")
}

func handleAdd(database *db.DB) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	name := fs.String("name", "", "Name of the feed")
	url := fs.String("url", "", "URL of the feed")
	fs.Parse(os.Args[2:])

	if *name == "" || *url == "" {
		fmt.Println("Missing required flags: --name and --url")
		os.Exit(1)
	}

	feed := models.Feed{
		Name: *name,
		URL:  *url,
	}

	err := database.AddFeed(&feed)
	if err != nil {
		fmt.Printf("Error adding feed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Feed added: %s (%s)\n", *name, *url)
}

func handleList(database *db.DB) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	num := fs.Int("num", 0, "Number of feeds to show (default: all)")
	fs.Parse(os.Args[2:])

	feeds, err := database.ListFeeds(*num)
	if err != nil {
		fmt.Printf("Error listing feeds: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("# Available RSS Feeds")
	for i, feed := range feeds {
		fmt.Printf("%d. Name: %s\n   URL: %s\n   Added: %s\n\n", i+1, feed.Name, feed.URL, feed.CreatedAt.Format("2006-01-02 15:04"))
	}
}

func handleDelete(database *db.DB) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	name := fs.String("name", "", "Name of the feed to delete")
	fs.Parse(os.Args[2:])

	if *name == "" {
		fmt.Println("Missing required flag: --name")
		os.Exit(1)
	}

	err := database.DeleteFeed(*name)
	if err != nil {
		fmt.Printf("Error deleting feed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Feed deleted: %s\n", *name)
}

func handleArticles(database *db.DB) {
	fs := flag.NewFlagSet("articles", flag.ExitOnError)
	feedName := fs.String("feed-name", "", "Name of the feed")
	num := fs.Int("num", 3, "Number of articles to show")
	fs.Parse(os.Args[2:])

	if *feedName == "" {
		fmt.Println("Missing required flag: --feed-name")
		os.Exit(1)
	}

	articles, err := database.GetArticles(*feedName, *num)
	if err != nil {
		fmt.Printf("Error getting articles: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Feed: %s\n\n", *feedName)
	for i, art := range articles {
		fmt.Printf("%d. [%s] %s\n   %s\n\n", i+1, art.PublishedAt.Format("2006-01-02"), art.Title, art.Link)
	}
}

func handleSetInterval() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: rsshub set-interval <duration> (e.g., 2m)")
		os.Exit(1)
	}
	durStr := os.Args[2]

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Println("Background process is not running")
		os.Exit(1)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("set-interval " + durStr + "\n"))
	if err != nil {
		fmt.Printf("Error sending command: %v\n", err)
		os.Exit(1)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(string(buf[:n]))
}

func handleSetWorkers() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: rsshub set-workers <count> (e.g., 5)")
		os.Exit(1)
	}
	countStr := os.Args[2]

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Println("Background process is not running")
		os.Exit(1)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("set-workers " + countStr + "\n"))
	if err != nil {
		fmt.Printf("Error sending command: %v\n", err)
		os.Exit(1)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(string(buf[:n]))
}

func printHelp() {
	fmt.Println(`Usage:
  rsshub COMMAND [OPTIONS]

  Common Commands:
     add             add new RSS feed
     set-interval    set RSS fetch interval
     set-workers     set number of workers
     list            list available RSS feeds
     delete          delete RSS feed
     articles        show latest articles
     fetch           starts the background process that periodically fetches and processes RSS feeds using a worker pool
`)
}
