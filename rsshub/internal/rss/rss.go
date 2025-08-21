package rss

import (
	"encoding/xml"
	"io"
	"net/http"
	"rsshub/internal/models"
)

func FetchAndParse(url string) (*models.RSSFeed, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var feed models.RSSFeed
	err = xml.Unmarshal(body, &feed)
	if err != nil {
		return nil, err
	}
	return &feed, nil
}
