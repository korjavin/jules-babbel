package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	oauth2v2 "google.golang.org/api/oauth2/v2"
	"golang.org/x/time/rate"
)

type GenerateRequest struct {
	TopicID string `json:"topic_id"`
}

type Topic struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prompt    string    `json:"prompt"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PromptVersion struct {
	ID        string    `json:"id"`
	TopicID   string    `json:"topic_id"`
	Prompt    string    `json:"prompt"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}

type Exercise struct {
	ID           string    `json:"id"`
	TopicID      string    `json:"topic_id"`
	PromptHash   string    `json:"prompt_hash"`
	ExerciseJSON string    `json:"exercise_json"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserExerciseView struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	ExerciseID        string    `json:"exercise_id"`
	LastViewed        time.Time `json:"last_viewed"`
	RepetitionCounter int       `json:"repetition_counter"`
}

type TopicRequest struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type User struct {
	ID       string `json:"id"`
	GoogleID string `json:"google_id"`
}

type UserStats struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	TotalExercises int    `json:"total_exercises"`
	TotalMistakes  int    `json:"total_mistakes"`
	TotalHints     int    `json:"total_hints"`
	TotalTime      int    `json:"total_time"`
	LastTopicID    string `json:"last_topic_id"`
}

type UpdateTopicRequest struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type OpenAIRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Database configuration
var (
	db *sql.DB

	// For observability
	lastRefinedPrompt      string
	lastRefinedPromptMutex sync.RWMutex
)

// Google OAuth2 configuration
var (
	googleOauthConfig *oauth2.Config
	oauthStateString  string
	googleAdminID     string
)

// Rate limiting
var (
	clients = make(map[string]*client)
	mu      sync.Mutex
)

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func getClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		return ip
	}
	ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	return ip
}

const metaPrompt = `You are a prompt engineering assistant. Your task is to refine the following user-provided prompt to improve the variety and creativity of the AI's output for generating language exercises.

**Refinement Rules:**
1.  **Do Not Change the JSON Schema:** The core instructions for the JSON output format and the schema definition must remain untouched. The refined prompt must still produce a valid JSON object.
2.  **Enhance Instructions:** Rephrase the instructions to encourage more diverse and less repetitive sentences. Add suggestions for using a wider range of vocabulary or sentence structures.
3.  **Add Examples:** Include one or two new, concrete examples of the desired output format within the prompt. This helps the model better understand the task.
4.  **Maintain Core Task:** The fundamental goal of the prompt (e.g., creating German conjunction exercises) must be preserved.
5.  **Output:** Your final output should be ONLY the refined prompt, with no extra text, explanations, or markdown formatting around it.

Here is the prompt to refine:
---
%s
---
`

// Initialize SQLite database
func initStorage() {
	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		log.Fatal("DATABASE_PATH environment variable is required")
	}

	log.Printf("Initializing SQLite database at %s", databasePath)
	var err error
	db, err = sql.Open("sqlite3", databasePath+"?_journal_mode=WAL") // Use WAL mode for better concurrency
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	db.SetMaxOpenConns(1)

	// Enable foreign keys
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		log.Fatalf("Failed to enable foreign keys: %v", err)
	}

	runMigrations(databasePath)
	log.Println("Database initialized successfully")
}

func runMigrations(databasePath string) {
	log.Println("Running database migrations...")
	m, err := migrate.New(
		"file://migrations",
		fmt.Sprintf("sqlite3://%s", databasePath),
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("Migrations applied successfully")
}

// Initialize with default topics
func initializeDefaultTopics() {
	log.Println("Checking for default topics...")
	existingTopics, err := getAllTopics()
	if err != nil {
		log.Printf("Warning: Could not check existing topics: %v", err)
		log.Printf("Attempting to create default topics anyway...")
	} else if len(existingTopics) > 0 {
		log.Printf("Found %d existing topics, skipping default topic initialization", len(existingTopics))
		return
	}

	defaultTopics := []struct {
		name   string
		prompt string
	}{
		{
			name: "Conjunctions",
			prompt: `You are an expert German language tutor creating B1-level grammar exercises. Your task is to generate a JSON object containing unique sentences focused on German conjunctions.

Please adhere to the following rules:
1. **Sentence Structure:** Each sentence must correctly use a German conjunction. Include a mix of coordinating and subordinating conjunctions from the provided list.
2. **Vocabulary:** Use common B1-level vocabulary.
3. **Clarity:** The English hint must be a natural and accurate translation of the German sentence.
Conjunction List: weil, obwohl, damit, wenn, dass, als, bevor, nachdem, ob, seit, und, oder, aber, denn, sondern.

Return ONLY the JSON object, with no other text or explanations.`,
		},
		{
			name: "Verb + Preposition",
			prompt: `You are an expert German language tutor creating B1-level exercises focused on German verbs with prepositions. Your task is to generate a JSON object containing unique sentences that practice verb-preposition combinations.

Please adhere to the following rules:
1. **Sentence Structure:** Each sentence must correctly use a German verb with its required preposition.
2. **Vocabulary:** Use common B1-level vocabulary.
3. **Clarity:** The English hint must be a natural and accurate translation of the German sentence.
Common verb-preposition combinations: denken an, warten auf, sich freuen über, sprechen über, bitten um, sich interessieren für, etc.

Return ONLY the JSON object, with no other text or explanations.`,
		},
		{
			name: "Preterite vs Perfect",
			prompt: `You are an expert German language tutor creating B1-level exercises focused on the correct usage of Preterite (Präteritum) vs Perfect tense (Perfekt) in German. Your task is to generate a JSON object containing unique sentences that practice these tenses.

Please adhere to the following rules:
1. **Sentence Structure:** Each sentence must demonstrate the appropriate use of either Preterite or Perfect tense.
2. **Vocabulary:** Use common B1-level vocabulary.
3. **Clarity:** The English hint must be a natural and accurate translation of the German sentence.
Focus on: written vs spoken contexts, completed actions, narrative vs conversational style.

Return ONLY the JSON object, with no other text or explanations.`,
		},
	}

	log.Printf("Initializing %d default topics...", len(defaultTopics))
	for _, defaultTopic := range defaultTopics {
		topic, err := createTopic(defaultTopic.name, defaultTopic.prompt)
		if err != nil {
			log.Printf("Error creating default topic '%s': %v", defaultTopic.name, err)
		} else {
			log.Printf("Created default topic: %s (ID: %s)", topic.Name, topic.ID)
		}
	}
}

// Data access functions using SQLite
func createTopic(name, prompt string) (*Topic, error) {
	log.Println("DB: createTopic")
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	topic := &Topic{
		ID:        uuid.New().String(),
		Name:      name,
		Prompt:    prompt,
		CreatedAt: now,
		UpdatedAt: now,
	}

	stmt, err := tx.Prepare("INSERT INTO topics(id, name, prompt, created_at, updated_at) VALUES(?, ?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(topic.ID, topic.Name, topic.Prompt, topic.CreatedAt, topic.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Create initial version
	if err := addPromptVersion(tx, topic.ID, prompt); err != nil {
		// Log but don't fail the transaction
		log.Printf("Warning: Failed to create initial version for new topic %s: %v", topic.ID, err)
	}

	return topic, tx.Commit()
}

func getAllTopics() ([]*Topic, error) {
	log.Println("DB: getAllTopics")
	rows, err := db.Query("SELECT id, name, prompt, created_at, updated_at FROM topics ORDER BY created_at ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []*Topic
	for rows.Next() {
		var topic Topic
		if err := rows.Scan(&topic.ID, &topic.Name, &topic.Prompt, &topic.CreatedAt, &topic.UpdatedAt); err != nil {
			return nil, err
		}
		topics = append(topics, &topic)
	}
	return topics, nil
}

func getTopic(topicID string) (*Topic, error) {
	log.Printf("DB: getTopic (id: %s)", topicID)
	return getTopicTx(nil, topicID)
}

func getTopicTx(tx *sql.Tx, topicID string) (*Topic, error) {
	var topic Topic
	query := "SELECT id, name, prompt, created_at, updated_at FROM topics WHERE id = ?"
	var err error
	if tx != nil {
		err = tx.QueryRow(query, topicID).Scan(&topic.ID, &topic.Name, &topic.Prompt, &topic.CreatedAt, &topic.UpdatedAt)
	} else {
		err = db.QueryRow(query, topicID).Scan(&topic.ID, &topic.Name, &topic.Prompt, &topic.CreatedAt, &topic.UpdatedAt)
	}

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("topic not found")
		}
		return nil, err
	}
	return &topic, nil
}

func updateTopic(topicID, name, prompt string) (*Topic, error) {
	log.Printf("DB: updateTopic (id: %s)", topicID)
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// First add the new version
	if err := addPromptVersion(tx, topicID, prompt); err != nil {
		log.Printf("Warning: Failed to create version during topic update: %v", err)
	}

	// Clean up old versions (keep only last 10)
	versions, err := getVersionsTx(tx, topicID)
	if err == nil && len(versions) > 10 {
		oldVersions := versions[:len(versions)-10]
		var oldVersionIDs []string
		for _, oldVersion := range oldVersions {
			oldVersionIDs = append(oldVersionIDs, oldVersion.ID)
		}
		// In-place construction of the query with placeholders
		query := "DELETE FROM prompt_versions WHERE id IN (?" + strings.Repeat(",?", len(oldVersionIDs)-1) + ")"
		stmt, err := tx.Prepare(query)
		if err == nil {
			args := make([]interface{}, len(oldVersionIDs))
			for i, id := range oldVersionIDs {
				args[i] = id
			}
			_, err = stmt.Exec(args...)
			if err != nil {
				log.Printf("Warning: Failed to delete old versions: %v", err)
			}
			stmt.Close()
		}
	}

	// Update topic
	now := time.Now()
	stmt, err := tx.Prepare("UPDATE topics SET name = ?, prompt = ?, updated_at = ? WHERE id = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if _, err := stmt.Exec(name, prompt, now, topicID); err != nil {
		return nil, err
	}

	// Get the updated topic data within the same transaction
	updatedTopic, err := getTopicTx(tx, topicID)
	if err != nil {
		return nil, err
	}

	return updatedTopic, tx.Commit()
}

func deleteTopic(topicID string) error {
	log.Printf("DB: deleteTopic (id: %s)", topicID)
	stmt, err := db.Prepare("DELETE FROM topics WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(topicID)
	return err
}

func getVersions(topicID string) ([]*PromptVersion, error) {
	log.Printf("DB: getVersions (topicID: %s)", topicID)
	return getVersionsTx(nil, topicID)
}

func getVersionsTx(tx *sql.Tx, topicID string) ([]*PromptVersion, error) {
	var rows *sql.Rows
	var err error
	query := "SELECT id, topic_id, prompt, version, created_at FROM prompt_versions WHERE topic_id = ? ORDER BY version ASC"

	if tx != nil {
		rows, err = tx.Query(query, topicID)
	} else {
		rows, err = db.Query(query, topicID)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*PromptVersion
	for rows.Next() {
		var v PromptVersion
		if err := rows.Scan(&v.ID, &v.TopicID, &v.Prompt, &v.Version, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, &v)
	}
	return versions, nil
}

func getVersion(versionID string) (*PromptVersion, error) {
	log.Printf("DB: getVersion (id: %s)", versionID)
	var v PromptVersion
	err := db.QueryRow("SELECT id, topic_id, prompt, version, created_at FROM prompt_versions WHERE id = ?", versionID).Scan(&v.ID, &v.TopicID, &v.Prompt, &v.Version, &v.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("version not found")
		}
		return nil, err
	}
	return &v, nil
}

func addPromptVersion(tx *sql.Tx, topicID, prompt string) error {
	log.Printf("DB: addPromptVersion (topicID: %s)", topicID)
	versions, err := getVersionsTx(tx, topicID)
	if err != nil {
		return err
	}

	nextVersion := 1
	if len(versions) > 0 {
		nextVersion = versions[len(versions)-1].Version + 1
	}

	version := &PromptVersion{
		ID:        uuid.New().String(),
		TopicID:   topicID,
		Prompt:    prompt,
		Version:   nextVersion,
		CreatedAt: time.Now(),
	}

	stmt, err := tx.Prepare("INSERT INTO prompt_versions(id, topic_id, prompt, version, created_at) VALUES(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(version.ID, version.TopicID, version.Prompt, version.Version, version.CreatedAt)
	return err
}

func getPromptHash(prompt string) string {
	hash := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(hash[:])
}

func createExercise(topicID, promptHash, exerciseJSON string) (*Exercise, error) {
	log.Printf("DB: createExercise (topicID: %s)", topicID)
	exercise := &Exercise{
		ID:           uuid.New().String(),
		TopicID:      topicID,
		PromptHash:   promptHash,
		ExerciseJSON: exerciseJSON,
		CreatedAt:    time.Now(),
	}

	stmt, err := db.Prepare("INSERT INTO exercises(id, topic_id, prompt_hash, exercise_json, created_at) VALUES(?, ?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(exercise.ID, exercise.TopicID, exercise.PromptHash, exercise.ExerciseJSON, exercise.CreatedAt)
	if err != nil {
		return nil, err
	}

	return exercise, nil
}

func getExercisesForTopic(topicID, promptHash string) ([]*Exercise, error) {
	log.Printf("DB: getExercisesForTopic (topicID: %s, promptHash: %s)", topicID, promptHash)
	rows, err := db.Query("SELECT id, topic_id, prompt_hash, exercise_json, created_at FROM exercises WHERE topic_id = ? AND prompt_hash = ?", topicID, promptHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exercises []*Exercise
	for rows.Next() {
		var ex Exercise
		if err := rows.Scan(&ex.ID, &ex.TopicID, &ex.PromptHash, &ex.ExerciseJSON, &ex.CreatedAt); err != nil {
			return nil, err
		}
		exercises = append(exercises, &ex)
	}
	return exercises, nil
}

func getUserExerciseViews(userID string) (map[string]*UserExerciseView, error) {
	log.Printf("DB: getUserExerciseViews (userID: %s)", userID)
	rows, err := db.Query("SELECT id, user_id, exercise_id, last_viewed, repetition_counter FROM user_exercise_views WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	views := make(map[string]*UserExerciseView)
	for rows.Next() {
		var view UserExerciseView
		if err := rows.Scan(&view.ID, &view.UserID, &view.ExerciseID, &view.LastViewed, &view.RepetitionCounter); err != nil {
			return nil, err
		}
		views[view.ExerciseID] = &view
	}
	return views, nil
}

func updateUserExerciseViews(viewsToUpdate []*UserExerciseView) error {
	log.Printf("DB: updateUserExerciseViews (count: %d)", len(viewsToUpdate))
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO user_exercise_views (id, user_id, exercise_id, last_viewed, repetition_counter)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_viewed = excluded.last_viewed,
			repetition_counter = excluded.repetition_counter
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, view := range viewsToUpdate {
		if view.ID == "" {
			view.ID = uuid.New().String() // Assign new ID for new views
		}
		_, err := stmt.Exec(view.ID, view.UserID, view.ExerciseID, view.LastViewed, view.RepetitionCounter)
		if err != nil {
			return fmt.Errorf("failed to upsert user exercise view: %w", err)
		}
	}

	return tx.Commit()
}

func initOAuth() {
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirectURL := os.Getenv("GOOGLE_REDIRECT_URL")

	if googleClientID == "" || googleClientSecret == "" || redirectURL == "" {
		log.Println("Warning: GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, or GOOGLE_REDIRECT_URL not set. Google login will be disabled.")
		googleOauthConfig = nil
		return
	}

	b := make([]byte, 16)
	rand.Read(b)
	oauthStateString = base64.URLEncoding.EncodeToString(b)

	googleOauthConfig = &oauth2.Config{
		RedirectURL:  redirectURL,
		ClientID:     googleClientID,
		ClientSecret: googleClientSecret,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}
	log.Println("Google OAuth initialized.")

	googleAdminID = os.Getenv("GOOGLE_ADMIN_ID")
	if googleAdminID == "" {
		log.Println("Warning: GOOGLE_ADMIN_ID not set. Admin features will be disabled.")
	} else {
		log.Println("Google Admin ID configured.")
	}
}

func main() {
	// Initialize storage backend
	initStorage()

	// Initialize Google OAuth
	initOAuth()

	// Initialize default topics
	initializeDefaultTopics()

	// Cleanup old clients every 10 minutes
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			mu.Lock()
			for ip, c := range clients {
				if time.Since(c.lastSeen) > 30*time.Minute {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Custom handler for index.html with cache-busting
	http.HandleFunc("/", handleIndex)

	// Serve static files with cache headers
	http.HandleFunc("/app.js", handleJS)
	http.HandleFunc("/privacy.html", handlePrivacy)
	http.HandleFunc("/favicon.svg", handleFavicon)
	http.HandleFunc("/favicon-32x32.svg", handleFavicon32)
	http.HandleFunc("/favicon.ico", handleFaviconICO) // Fallback for older browsers

	// API endpoints
	http.HandleFunc("/api/generate", handleGenerate) // Will be deprecated for frontend use
	http.HandleFunc("/api/exercises", handleExercises)
	http.HandleFunc("/api/topics", handleTopics)
	http.HandleFunc("/api/topics/", handleTopicByID)
	http.HandleFunc("/api/versions/", handleVersions)
	http.HandleFunc("/api/last-refined-prompt", handleGetLastRefinedPrompt)

	// Auth endpoints
	http.HandleFunc("/auth/google/login", handleGoogleLogin)
	http.HandleFunc("/auth/google/callback", handleGoogleCallback)
	http.HandleFunc("/api/auth/status", handleAuthStatus)
	http.HandleFunc("/auth/logout", handleLogout)
	http.HandleFunc("/api/auth/is_admin", handleIsAdmin)

	// User stats and settings endpoints
	http.HandleFunc("/api/user/stats", handleUserStats)
	http.HandleFunc("/api/user/settings", handleUserSettings)

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func getFilePath(filename string) string {
	// Check if running in Docker (files in static/ directory)
	if _, err := os.Stat("static/" + filename); err == nil {
		return "static/" + filename
	}
	// Otherwise use current directory
	return filename
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Read the HTML file
	filePath := getFilePath("index.html")
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Replace the cache-busting parameter with current timestamp
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	htmlContent := string(content)
	htmlContent = strings.ReplaceAll(htmlContent, "app.js?v=20250821001", fmt.Sprintf("app.js?v=%s", timestamp))

	// Set headers to prevent caching of the HTML file itself
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	w.Write([]byte(htmlContent))
}

func handleJS(w http.ResponseWriter, r *http.Request) {
	// Read the JS file
	filePath := getFilePath("app.js")
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Set headers for JS file - allow caching but with versioning
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year

	w.Write(content)
}

func handlePrivacy(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, getFilePath("privacy.html"))
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	// Read the SVG file
	filePath := getFilePath("favicon.svg")
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "Favicon not found", http.StatusNotFound)
		return
	}

	// Set headers for SVG favicon - allow caching
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year

	w.Write(content)
}

func handleFavicon32(w http.ResponseWriter, r *http.Request) {
	// Read the 32x32 SVG file
	filePath := getFilePath("favicon-32x32.svg")
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "Favicon not found", http.StatusNotFound)
		return
	}

	// Set headers for SVG favicon - allow caching
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year

	w.Write(content)
}

func handleFaviconICO(w http.ResponseWriter, r *http.Request) {
	// Redirect .ico requests to SVG favicon for better quality
	http.Redirect(w, r, "/favicon.svg", http.StatusMovedPermanently)
}

// refinePrompt takes a prompt and uses the meta-prompt to refine it.
func refinePrompt(originalPrompt, apiKey, openaiURL, modelName string) (string, error) {
	log.Println("Refining prompt...")

	// 1. Create the request to refine the prompt
	refineMessages := []Message{
		{
			Role:    "user",
			Content: fmt.Sprintf(metaPrompt, originalPrompt),
		},
	}

	// For refining, we expect a text response, not a JSON object
	refineReq := OpenAIRequest{
		Model:    modelName,
		Messages: refineMessages,
	}

	reqBody, err := json.Marshal(refineReq)
	if err != nil {
		return "", fmt.Errorf("failed to create refine request body: %w", err)
	}

	// 2. Make the request to the OpenAI API
	client := &http.Client{}
	apiReq, err := http.NewRequest("POST", openaiURL+"/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create API request for refining: %w", err)
	}
	apiReq.Header.Set("Content-Type", "application/json")
	apiReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(apiReq)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI API for refining: %w", err)
	}
	defer resp.Body.Close()

	// 3. Read and parse the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read API response for refining: %w", err)
	}

	var openaiResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		return "", fmt.Errorf("failed to parse API response for refining: %w", err)
	}

	if openaiResp.Error != nil {
		return "", fmt.Errorf("API error during refining: %s", openaiResp.Error.Message)
	}

	if len(openaiResp.Choices) == 0 || openaiResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("received an empty response from the refining API")
	}

	refinedPrompt := openaiResp.Choices[0].Message.Content
	log.Println("Successfully refined prompt.")
	return refinedPrompt, nil
}

func handleExercises(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleExercises (%s)", r.Method)
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	log.Printf("API: handleExercises - TopicID: %s", req.TopicID)

	topic, err := getTopic(req.TopicID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Topic not found: %v", err), http.StatusNotFound)
		return
	}

	promptHash := getPromptHash(topic.Prompt)
	userID := getUserIDFromRequest(r)

	allExercises, err := getExercisesForTopic(req.TopicID, promptHash)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get exercises: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("Found %d cached exercises for topic %s", len(allExercises), req.TopicID)

	var finalExercises []*Exercise
	if userID == "" {
		log.Println("Guest user - serving from cache.")
		finalExercises = getRandomExercises(allExercises, 10)
	} else {
		log.Printf("Authenticated user (ID: %s) - applying SRS logic.", userID)
		userViews, err := getUserExerciseViews(userID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get user views: %v", err), http.StatusInternalServerError)
			return
		}

		eligibleExercises := getEligibleExercisesForSRS(allExercises, userViews)
		log.Printf("%d exercises are eligible for SRS review.", len(eligibleExercises))

		if len(eligibleExercises) < 10 {
			log.Println("Cache insufficient, generating new exercises.")
			newlyGenerated, err := generateAndCacheExercises(topic)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to generate exercises: %v", err), http.StatusInternalServerError)
				return
			}
			log.Printf("Generated %d new exercises.", len(newlyGenerated))
			allExercises = append(allExercises, newlyGenerated...)
			eligibleExercises = getEligibleExercisesForSRS(allExercises, userViews)
		}

		finalExercises = getRandomExercises(eligibleExercises, 10)

		// Update views for the selected exercises
		var viewsToUpdate []*UserExerciseView
		now := time.Now()
		for _, ex := range finalExercises {
			view, exists := userViews[ex.ID]
			if !exists {
				view = &UserExerciseView{
					UserID:     userID,
					ExerciseID: ex.ID,
				}
			}
			view.LastViewed = now
			view.RepetitionCounter++
			viewsToUpdate = append(viewsToUpdate, view)
		}
		if err := updateUserExerciseViews(viewsToUpdate); err != nil {
			log.Printf("Warning: failed to update user exercise views: %v", err)
		}
	}

	// Prepare response
	var responseExercises []json.RawMessage
	for _, ex := range finalExercises {
		responseExercises = append(responseExercises, []byte(ex.ExerciseJSON))
	}

	log.Printf("API: handleExercises - Responding with %d exercises.", len(responseExercises))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]json.RawMessage{"exercises": responseExercises})
}

func generateAndCacheExercises(topic *Topic) ([]*Exercise, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	openaiURL := os.Getenv("OPENAI_URL")
	if openaiURL == "" {
		openaiURL = "https://api.openai.com/v1"
	}
	modelName := os.Getenv("MODEL_NAME")
	if modelName == "" {
		modelName = "gpt-3.5-turbo-1106"
	}

	finalPrompt, err := refinePrompt(topic.Prompt, apiKey, openaiURL, modelName)
	if err != nil {
		log.Printf("Error refining prompt, falling back to original: %v", err)
		finalPrompt = topic.Prompt
	} else {
		lastRefinedPromptMutex.Lock()
		lastRefinedPrompt = finalPrompt
		lastRefinedPromptMutex.Unlock()
	}

	openaiReq := OpenAIRequest{
		Model:          modelName,
		Messages:       []Message{{Role: "user", Content: finalPrompt}},
		ResponseFormat: &ResponseFormat{Type: "json_object"},
	}

	reqBody, _ := json.Marshal(openaiReq)
	client := &http.Client{}
	apiReq, _ := http.NewRequest("POST", openaiURL+"/chat/completions", bytes.NewBuffer(reqBody))
	apiReq.Header.Set("Content-Type", "application/json")
	apiReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var openaiResp OpenAIResponse
	json.Unmarshal(respBody, &openaiResp)

	if openaiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", openaiResp.Error.Message)
	}

	if len(openaiResp.Choices) == 0 || openaiResp.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("received an empty response from OpenAI")
	}

	// The actual content is a JSON string inside the response.
	var exerciseData struct {
		Exercises []json.RawMessage `json:"exercises"`
	}
	if err := json.Unmarshal([]byte(openaiResp.Choices[0].Message.Content), &exerciseData); err != nil {
		return nil, fmt.Errorf("failed to parse exercises from OpenAI response: %w", err)
	}

	promptHash := getPromptHash(topic.Prompt)
	var newlyGenerated []*Exercise
	for _, exJSON := range exerciseData.Exercises {
		exercise, err := createExercise(topic.ID, promptHash, string(exJSON))
		if err != nil {
			log.Printf("Warning: failed to cache exercise: %v", err)
			continue
		}
		newlyGenerated = append(newlyGenerated, exercise)
	}

	return newlyGenerated, nil
}

func getEligibleExercisesForSRS(allExercises []*Exercise, userViews map[string]*UserExerciseView) []*Exercise {
	var eligible []*Exercise
	now := time.Now()
	for _, ex := range allExercises {
		view, seen := userViews[ex.ID]
		if !seen {
			eligible = append(eligible, ex)
			continue
		}
		// SRS logic: next review date is (counter^2) days after last view
		daysSinceView := now.Sub(view.LastViewed).Hours() / 24
		nextReviewInDays := float64(view.RepetitionCounter * view.RepetitionCounter)
		if daysSinceView >= nextReviewInDays {
			eligible = append(eligible, ex)
		}
	}
	return eligible
}

func getRandomExercises(exercises []*Exercise, count int) []*Exercise {
	if len(exercises) <= count {
		return exercises
	}
	mrand.Shuffle(len(exercises), func(i, j int) {
		exercises[i], exercises[j] = exercises[j], exercises[i]
	})
	return exercises[:count]
}

func getUserIDFromRequest(r *http.Request) string {
	cookie, err := r.Cookie("user_id")
	if err != nil {
		return "" // No cookie, so not logged in
	}
	return cookie.Value
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleGenerate (%s)", r.Method)
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Enable CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Rate limiting
	ip := getClientIP(r)
	mu.Lock()
	if _, found := clients[ip]; !found {
		// Allow 1 request every 3 seconds, with a burst of 1.
		clients[ip] = &client{limiter: rate.NewLimiter(rate.Every(3*time.Second), 1)}
	}
	clients[ip].lastSeen = time.Now()
	if !clients[ip].limiter.Allow() {
		mu.Unlock()
		// Return a JSON error message
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "You are making requests too quickly. Please wait a few seconds and try again.",
			},
		})
		return
	}
	mu.Unlock()

	// Get configuration from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		http.Error(w, "OpenAI API key not configured", http.StatusInternalServerError)
		return
	}

	openaiURL := os.Getenv("OPENAI_URL")
	if openaiURL == "" {
		openaiURL = "https://api.openai.com/v1"
	}

	modelName := os.Getenv("MODEL_NAME")
	if modelName == "" {
		modelName = "gpt-3.5-turbo-1106"
	}

	// Parse request
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get topic and its prompt
	topic, err := getTopic(req.TopicID)
	if err != nil {
		http.Error(w, "Topic not found", http.StatusNotFound)
		return
	}

	// Refine the prompt
	finalPrompt, err := refinePrompt(topic.Prompt, apiKey, openaiURL, modelName)
	if err != nil {
		// If refining fails, log the error and fall back to the original prompt
		log.Printf("Error refining prompt, falling back to original: %v", err)
		finalPrompt = topic.Prompt
	} else {
		// Store the last successfully refined prompt for observability
		lastRefinedPromptMutex.Lock()
		lastRefinedPrompt = finalPrompt
		lastRefinedPromptMutex.Unlock()
	}

	// Create OpenAI request with the (potentially refined) prompt
	openaiReq := OpenAIRequest{
		Model: modelName,
		Messages: []Message{
			{
				Role:    "user",
				Content: finalPrompt,
			},
		},
		ResponseFormat: &ResponseFormat{Type: "json_object"},
	}

	// Marshal request
	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Make request to OpenAI API
	client := &http.Client{}
	apiReq, err := http.NewRequest("POST", openaiURL+"/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		http.Error(w, "Failed to create API request", http.StatusInternalServerError)
		return
	}

	apiReq.Header.Set("Content-Type", "application/json")
	apiReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(apiReq)
	if err != nil {
		http.Error(w, "Failed to call OpenAI API", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read API response", http.StatusInternalServerError)
		return
	}

	// Parse response to check for errors
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		http.Error(w, "Failed to parse API response", http.StatusInternalServerError)
		return
	}

	// Check for API errors
	if openaiResp.Error != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		errorResp := map[string]any{
			"error": map[string]string{
				"message": openaiResp.Error.Message,
				"type":    openaiResp.Error.Type,
			},
		}
		json.NewEncoder(w).Encode(errorResp)
		return
	}

	// Forward successful response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func handleGetLastRefinedPrompt(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleGetLastRefinedPrompt (%s)", r.Method)
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	lastRefinedPromptMutex.RLock()
	defer lastRefinedPromptMutex.RUnlock()

	response := map[string]string{
		"last_refined_prompt": lastRefinedPrompt,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// Handle topics CRUD operations
func handleTopics(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleTopics (%s)", r.Method)
	// Enable CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		topicsList, err := getAllTopics()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get topics: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]*Topic{"topics": topicsList})

	case http.MethodPost:
		adminOnly(func(w http.ResponseWriter, r *http.Request) {
			var req TopicRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			if req.Name == "" || req.Prompt == "" {
				http.Error(w, "Name and prompt are required", http.StatusBadRequest)
				return
			}

			topic, err := createTopic(req.Name, req.Prompt)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to create topic: %v", err), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(topic)
		}).ServeHTTP(w, r)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleUserStats(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleUserStats (%s)", r.Method)
	cookie, err := r.Cookie("user_id")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	userID := cookie.Value

	switch r.Method {
	case http.MethodGet:
		stats, err := getUserStats(userID)
		if err != nil {
			http.Error(w, "Failed to get user stats", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(stats)
	case http.MethodPost:
		var stats UserStats
		if err := json.NewDecoder(r.Body).Decode(&stats); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		stats.UserID = userID
		if err := updateUserStats(&stats); err != nil {
			http.Error(w, "Failed to update user stats", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleUserSettings(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleUserSettings (%s)", r.Method)
	cookie, err := r.Cookie("user_id")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	userID := cookie.Value

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var settings struct {
		LastTopicID string `json:"last_topic_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := updateUserSetting(userID, settings.LastTopicID); err != nil {
		http.Error(w, "Failed to update user settings", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func getUserByGoogleID(googleID string) (*User, error) {
	log.Printf("DB: getUserByGoogleID (googleID: %s)", googleID)
	var user User
	err := db.QueryRow("SELECT id, google_id FROM users WHERE google_id = ?", googleID).Scan(&user.ID, &user.GoogleID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, err
	}
	return &user, nil
}

func createUser(googleID string) (*User, error) {
	log.Printf("DB: createUser (googleID: %s)", googleID)
	user := &User{
		ID:       uuid.New().String(),
		GoogleID: googleID,
	}

	stmt, err := db.Prepare("INSERT INTO users(id, google_id) VALUES(?, ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(user.ID, user.GoogleID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func getUserStats(userID string) (*UserStats, error) {
	log.Printf("DB: getUserStats (userID: %s)", userID)
	var stats UserStats
	// Use COALESCE to handle NULL last_topic_id
	err := db.QueryRow("SELECT id, user_id, total_exercises, total_mistakes, total_hints, total_time, COALESCE(last_topic_id, '') FROM user_stats WHERE user_id = ?", userID).Scan(&stats.ID, &stats.UserID, &stats.TotalExercises, &stats.TotalMistakes, &stats.TotalHints, &stats.TotalTime, &stats.LastTopicID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &UserStats{UserID: userID}, nil // Return empty stats if not found
		}
		return nil, err
	}
	return &stats, nil
}

func updateUserStats(stats *UserStats) error {
	log.Printf("DB: updateUserStats (userID: %s)", stats.UserID)
	if stats.ID == "" {
		stats.ID = uuid.New().String()
	}

	stmt, err := db.Prepare(`
		INSERT INTO user_stats (id, user_id, total_exercises, total_mistakes, total_hints, total_time, last_topic_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			total_exercises = excluded.total_exercises,
			total_mistakes = excluded.total_mistakes,
			total_hints = excluded.total_hints,
			total_time = excluded.total_time
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(stats.ID, stats.UserID, stats.TotalExercises, stats.TotalMistakes, stats.TotalHints, stats.TotalTime, stats.LastTopicID)
	return err
}

func updateUserSetting(userID, lastTopicID string) error {
	log.Printf("DB: updateUserSetting (userID: %s, lastTopicID: %s)", userID, lastTopicID)
	stmt, err := db.Prepare(`
        INSERT INTO user_stats (id, user_id, last_topic_id, total_exercises, total_mistakes, total_hints, total_time)
        VALUES (?, ?, ?, 0, 0, 0, 0)
        ON CONFLICT(user_id) DO UPDATE SET
            last_topic_id = excluded.last_topic_id
    `)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(uuid.New().String(), userID, lastTopicID)
	return err
}

func handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleGoogleLogin (%s)", r.Method)
	if googleOauthConfig == nil {
		http.Error(w, "Google login is not configured", http.StatusInternalServerError)
		return
	}
	url := googleOauthConfig.AuthCodeURL(oauthStateString)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleGoogleCallback (%s)", r.Method)
	if googleOauthConfig == nil {
		http.Error(w, "Google login is not configured", http.StatusInternalServerError)
		return
	}

	state := r.FormValue("state")
	if state != oauthStateString {
		log.Printf("Invalid oauth state, expected '%s', got '%s'\n", oauthStateString, state)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	code := r.FormValue("code")
	token, err := googleOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("oauthConf.Exchange() failed with '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	oauth2Client := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(token))
	oauth2Service, err := oauth2v2.New(oauth2Client)
	if err != nil {
		log.Printf("Unable to create oauth2 service: %v", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	userinfo, err := oauth2Service.Userinfo.Get().Do()
	if err != nil {
		log.Printf("Unable to get user info: %v", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	user, err := getUserByGoogleID(userinfo.Id)
	if err != nil {
		log.Printf("Unable to get user by google ID: %v", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	if user == nil {
		log.Printf("User with googleID %s not found, creating new user.", userinfo.Id)
		user, err = createUser(userinfo.Id)
		if err != nil {
			log.Printf("Unable to create user: %v", err)
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
	}

	log.Printf("Setting user_id cookie for user %s", user.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     "user_id",
		Value:    user.ID,
		HttpOnly: true,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour), // 30 days
	})

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleAuthStatus (%s)", r.Method)
	cookie, err := r.Cookie("user_id")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"logged_in": false})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"logged_in": true, "user_id": cookie.Value})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleLogout (%s)", r.Method)
	http.SetCookie(w, &http.Cookie{
		Name:     "user_id",
		Value:    "",
		HttpOnly: true,
		Path:     "/",
		Expires:  time.Unix(0, 0),
	})
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func handleIsAdmin(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleIsAdmin (%s)", r.Method)
	w.Header().Set("Content-Type", "application/json")

	isAdmin := false
	if googleAdminID != "" {
		userID := getUserIDFromRequest(r)
		if userID != "" {
			user, err := getUserByID(userID)
			if err == nil && user != nil && user.GoogleID == googleAdminID {
				isAdmin = true
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]bool{"is_admin": isAdmin})
}

func getUserByID(userID string) (*User, error) {
	log.Printf("DB: getUserByID (userID: %s)", userID)
	var user User
	err := db.QueryRow("SELECT id, google_id FROM users WHERE id = ?", userID).Scan(&user.ID, &user.GoogleID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, err
	}
	return &user, nil
}

func adminOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if googleAdminID == "" {
			http.Error(w, "Admin features are not configured", http.StatusForbidden)
			return
		}

		userID := getUserIDFromRequest(r)
		if userID == "" {
			http.Error(w, "You must be logged in to perform this action", http.StatusUnauthorized)
			return
		}

		user, err := getUserByID(userID)
		if err != nil || user == nil {
			log.Printf("Error getting user for admin check (userID: %s): %v", userID, err)
			http.Error(w, "Could not verify user credentials", http.StatusInternalServerError)
			return
		}

		if user.GoogleID != googleAdminID {
			log.Printf("Admin access denied for user (googleID: %s)", user.GoogleID)
			http.Error(w, "You do not have permission to perform this action", http.StatusForbidden)
			return
		}

		h.ServeHTTP(w, r)
	}
}

// Handle individual topic operations
func handleTopicByID(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleTopicByID (%s) - Path: %s", r.Method, r.URL.Path)
	// Enable CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Extract topic ID from path
	topicID := strings.TrimPrefix(r.URL.Path, "/api/topics/")
	if topicID == "" {
		http.Error(w, "Topic ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		topic, err := getTopic(topicID)
		if err != nil {
			http.Error(w, "Topic not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(topic)

	case http.MethodPut:
		adminOnly(func(w http.ResponseWriter, r *http.Request) {
			var req UpdateTopicRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			if req.Prompt == "" {
				http.Error(w, "Prompt is required", http.StatusBadRequest)
				return
			}
			if req.Name == "" {
				// Get current name if not provided
				currentTopic, err := getTopic(topicID)
				if err != nil {
					http.Error(w, "Topic not found", http.StatusNotFound)
					return
				}
				req.Name = currentTopic.Name
			}

			topic, err := updateTopic(topicID, req.Name, req.Prompt)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to update topic: %v", err), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(topic)
		}).ServeHTTP(w, r)

	case http.MethodDelete:
		adminOnly(func(w http.ResponseWriter, r *http.Request) {
			err := deleteTopic(topicID)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to delete topic: %v", err), http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusNoContent)
		}).ServeHTTP(w, r)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Handle prompt versions
func handleVersions(w http.ResponseWriter, r *http.Request) {
	log.Printf("API: handleVersions (%s) - Path: %s", r.Method, r.URL.Path)
	// Enable CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Extract topic ID from path: /api/versions/{topicID} or /api/versions/{topicID}/restore/{versionID}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/versions/"), "/")
	if len(pathParts) < 1 || pathParts[0] == "" {
		http.Error(w, "Topic ID required", http.StatusBadRequest)
		return
	}

	topicID := pathParts[0]

	switch r.Method {
	case http.MethodGet:
		versions, err := getVersions(topicID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get versions: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]*PromptVersion{"versions": versions})

	case http.MethodPost:
		adminOnly(func(w http.ResponseWriter, r *http.Request) {
			// Restore version: POST /api/versions/{topicID}/restore/{versionID}
			if len(pathParts) < 3 || pathParts[1] != "restore" {
				http.Error(w, "Invalid restore path", http.StatusBadRequest)
				return
			}

			versionID := pathParts[2]

			versionToRestore, err := getVersion(versionID)
			if err != nil {
				http.Error(w, "Version not found", http.StatusNotFound)
				return
			}

			// Verify the version belongs to the requested topic
			if versionToRestore.TopicID != topicID {
				http.Error(w, "Version does not belong to this topic", http.StatusBadRequest)
				return
			}

			// Get the current topic name to preserve it
			currentTopic, err := getTopic(topicID)
			if err != nil {
				http.Error(w, "Failed to get current topic", http.StatusNotFound)
				return
			}

			// Update topic with restored prompt (this will automatically create a new version)
			topic, err := updateTopic(topicID, currentTopic.Name, versionToRestore.Prompt)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to restore version: %v", err), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(topic)
		}).ServeHTTP(w, r)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}