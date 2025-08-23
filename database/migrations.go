package database

import "log"

func CreateTables() {
	_, err := DB.Exec(`
	CREATE TABLE IF NOT EXISTS topics (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		prompt TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS prompt_versions (
		id TEXT PRIMARY KEY,
		topic_id TEXT NOT NULL,
		prompt TEXT NOT NULL,
		version INTEGER NOT NULL,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (topic_id) REFERENCES topics(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS exercises (
		id TEXT PRIMARY KEY,
		topic_id TEXT NOT NULL,
		prompt_hash TEXT NOT NULL,
		exercise_json TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (topic_id) REFERENCES topics(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		google_id TEXT NOT NULL UNIQUE
	);

	CREATE TABLE IF NOT EXISTS user_stats (
		user_id TEXT PRIMARY KEY,
		total_exercises INTEGER NOT NULL DEFAULT 0,
		total_mistakes INTEGER NOT NULL DEFAULT 0,
		total_hints INTEGER NOT NULL DEFAULT 0,
		total_time INTEGER NOT NULL DEFAULT 0,
		last_topic_id TEXT,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS user_exercise_views (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		exercise_id TEXT NOT NULL,
		last_viewed DATETIME NOT NULL,
		repetition_counter INTEGER NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (exercise_id) REFERENCES exercises(id) ON DELETE CASCADE
	);
	`)

	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	log.Println("Database tables created or already exist.")
}
