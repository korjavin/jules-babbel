CREATE TABLE IF NOT EXISTS user_stats (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    total_exercises INTEGER NOT NULL,
    total_mistakes INTEGER NOT NULL,
    total_hints INTEGER NOT NULL,
    total_time INTEGER NOT NULL,
    last_topic_id TEXT,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
