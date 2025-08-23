CREATE TABLE IF NOT EXISTS prompt_versions (
    id TEXT PRIMARY KEY,
    topic_id TEXT NOT NULL,
    prompt TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY(topic_id) REFERENCES topics(id) ON DELETE CASCADE
);
