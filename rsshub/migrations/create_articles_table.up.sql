CREATE TABLE articles (
                          id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                          created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          updated_at TIMESTAMP,
                          title TEXT NOT NULL,
                          link TEXT NOT NULL,
                          published_at TIMESTAMP NOT NULL,
                          description TEXT,
                          feed_id UUID REFERENCES feeds(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX articles_feed_link_idx ON articles (feed_id, link);