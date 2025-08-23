CREATE TABLE IF NOT EXISTS exercises (
    id TEXT PRIMARY KEY,
    topic_id TEXT NOT NULL,
    prompt_hash TEXT NOT NULL,
    exercise_json TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY(topic_id) REFERENCES topics(id) ON DELETE CASCADE
);
