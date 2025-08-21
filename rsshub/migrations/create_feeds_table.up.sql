CREATE TABLE feeds (
                       id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                       created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                       updated_at TIMESTAMP,
                       name TEXT UNIQUE NOT NULL,
                       url TEXT NOT NULL
);