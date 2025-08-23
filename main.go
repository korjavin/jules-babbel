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

	"german-conjunctions-trainer/database"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/time/rate"
	goauth2 "google.golang.org/api/oauth2/v2"
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
	ID           string `json:"id"`
	TopicID      string `json:"topic_id"`
	PromptHash   string `json:"prompt_hash"`
	ExerciseJSON string `json:"exercise_json"`
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

var (
	lastRefinedPrompt      string
	lastRefinedPromptMutex sync.RWMutex
)

var (
	googleOauthConfig *oauth2.Config
	oauthStateString  string
	googleAdminID     string
)

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
	database.InitDB()
	database.LoadFixtures()

	initOAuth()

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

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/app.js", handleJS)
	http.HandleFunc("/privacy.html", handlePrivacy)
	http.HandleFunc("/favicon.svg", handleFavicon)
	http.HandleFunc("/favicon-32x32.svg", handleFavicon32)
	http.HandleFunc("/favicon.ico", handleFaviconICO)

	http.HandleFunc("/api/exercises", handleExercises)
	http.HandleFunc("/api/topics", handleTopics)
	http.HandleFunc("/api/topics/", handleTopicByID)
	http.HandleFunc("/api/versions/", handleVersions)
	http.HandleFunc("/api/last-refined-prompt", handleGetLastRefinedPrompt)

	http.HandleFunc("/auth/google/login", handleGoogleLogin)
	http.HandleFunc("/auth/google/callback", handleGoogleCallback)
	http.HandleFunc("/api/auth/status", handleAuthStatus)
	http.HandleFunc("/auth/logout", handleLogout)
	http.HandleFunc("/api/auth/is_admin", handleIsAdmin)

	http.HandleFunc("/api/user/stats", handleUserStats)
	http.HandleFunc("/api/user/settings", handleUserSettings)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func getFilePath(filename string) string {
	if _, err := os.Stat("static/" + filename); err == nil {
		return "static/" + filename
	}
	return filename
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	filePath := getFilePath("index.html")
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	htmlContent := string(content)
	htmlContent = strings.ReplaceAll(htmlContent, "app.js?v=20250821001", fmt.Sprintf("app.js?v=%s", timestamp))

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	w.Write([]byte(htmlContent))
}

func handleJS(w http.ResponseWriter, r *http.Request) {
	filePath := getFilePath("app.js")
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=31536000")

	w.Write(content)
}

func handlePrivacy(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, getFilePath("privacy.html"))
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	filePath := getFilePath("favicon.svg")
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "Favicon not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=31536000")

	w.Write(content)
}

func handleFavicon32(w http.ResponseWriter, r *http.Request) {
	filePath := getFilePath("favicon-32x32.svg")
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "Favicon not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=31536000")

	w.Write(content)
}

func handleFaviconICO(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/favicon.svg", http.StatusMovedPermanently)
}

func refinePrompt(originalPrompt, apiKey, openaiURL, modelName string) (string, error) {
	log.Println("Refining prompt...")

	refineMessages := []Message{
		{
			Role:    "user",
			Content: fmt.Sprintf(metaPrompt, originalPrompt),
		},
	}

	refineReq := OpenAIRequest{
		Model:    modelName,
		Messages: refineMessages,
	}

	reqBody, err := json.Marshal(refineReq)
	if err != nil {
		return "", fmt.Errorf("failed to create refine request body: %w", err)
	}

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

	var finalExercises []*Exercise
	if userID == "" {
		finalExercises = getRandomExercises(allExercises, 10)
	} else {
		userViews, err := getUserExerciseViews(userID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get user views: %v", err), http.StatusInternalServerError)
			return
		}

		eligibleExercises := getEligibleExercisesForSRS(allExercises, userViews)
		if len(eligibleExercises) < 10 {
			newlyGenerated, err := generateAndCacheExercises(topic)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to generate exercises: %v", err), http.StatusInternalServerError)
				return
			}
			allExercises = append(allExercises, newlyGenerated...)
			eligibleExercises = getEligibleExercisesForSRS(allExercises, userViews)
		}

		finalExercises = getRandomExercises(eligibleExercises, 10)

		var viewsToUpdate []*UserExerciseView
		now := time.Now()
		for _, ex := range finalExercises {
			view, exists := userViews[ex.ID]
			if !exists {
				view = &UserExerciseView{
					ID:         uuid.New().String(),
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

	var responseExercises []json.RawMessage
	for _, ex := range finalExercises {
		responseExercises = append(responseExercises, []byte(ex.ExerciseJSON))
	}

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
		return ""
	}
	return cookie.Value
}

func handleGetLastRefinedPrompt(w http.ResponseWriter, r *http.Request) {
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

func handleTopics(w http.ResponseWriter, r *http.Request) {
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
	row := database.DB.QueryRow("SELECT id, google_id FROM users WHERE google_id = ?", googleID)
	user := &User{}
	err := row.Scan(&user.ID, &user.GoogleID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func createUser(googleID string) (*User, error) {
	user := &User{
		ID:       uuid.New().String(),
		GoogleID: googleID,
	}
	_, err := database.DB.Exec("INSERT INTO users (id, google_id) VALUES (?, ?)", user.ID, user.GoogleID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func getUserStats(userID string) (*UserStats, error) {
	row := database.DB.QueryRow("SELECT total_exercises, total_mistakes, total_hints, total_time, last_topic_id FROM user_stats WHERE user_id = ?", userID)
	stats := &UserStats{UserID: userID}
	var lastTopicID sql.NullString
	err := row.Scan(&stats.TotalExercises, &stats.TotalMistakes, &stats.TotalHints, &stats.TotalTime, &lastTopicID)
	if err == sql.ErrNoRows {
		return stats, nil
	}
	if err != nil {
		return nil, err
	}
	if lastTopicID.Valid {
		stats.LastTopicID = lastTopicID.String
	}
	return stats, nil
}

func updateUserStats(stats *UserStats) error {
	_, err := database.DB.Exec(`
		INSERT INTO user_stats (user_id, total_exercises, total_mistakes, total_hints, total_time)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			total_exercises = total_exercises + excluded.total_exercises,
			total_mistakes = total_mistakes + excluded.total_mistakes,
			total_hints = total_hints + excluded.total_hints,
			total_time = total_time + excluded.total_time
	`, stats.UserID, stats.TotalExercises, stats.TotalMistakes, stats.TotalHints, stats.TotalTime)
	return err
}

func updateUserSetting(userID, lastTopicID string) error {
	_, err := database.DB.Exec(`
		INSERT INTO user_stats (user_id, last_topic_id)
		VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			last_topic_id = excluded.last_topic_id
	`, userID, lastTopicID)
	return err
}

func handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if googleOauthConfig == nil {
		http.Error(w, "Google login is not configured", http.StatusInternalServerError)
		return
	}
	url := googleOauthConfig.AuthCodeURL(oauthStateString)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
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
	oauth2Service, err := goauth2.New(oauth2Client)
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
		user, err = createUser(userinfo.Id)
		if err != nil {
			log.Printf("Unable to create user: %v", err)
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "user_id",
		Value:    user.ID,
		HttpOnly: true,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("user_id")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"logged_in": false})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"logged_in": true, "user_id": cookie.Value})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
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
	row := database.DB.QueryRow("SELECT id, google_id FROM users WHERE id = ?", userID)
	user := &User{}
	err := row.Scan(&user.ID, &user.GoogleID)
	if err == sql.ErrNoRows {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, err
	}
	return user, nil
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

func handleTopicByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

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

func handleVersions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/versions/"), "/")
	if len(pathParts) < 1 {
		http.Error(w, "Topic ID required", http.StatusBadRequest)
		return
	}
	topicID := pathParts[0]

	if len(pathParts) == 3 && pathParts[1] == "restore" {
		if r.Method == http.MethodPost {
			adminOnly(func(w http.ResponseWriter, r *http.Request) {
				versionID := pathParts[2]

				versionToRestore, err := getVersion(versionID)
				if err != nil {
					http.Error(w, "Version not found", http.StatusNotFound)
					return
				}

				currentTopic, err := getTopic(topicID)
				if err != nil {
					http.Error(w, "Topic not found", http.StatusNotFound)
					return
				}

				updatedTopic, err := updateTopic(topicID, currentTopic.Name, versionToRestore.Prompt)
				if err != nil {
					http.Error(w, fmt.Sprintf("Failed to restore version: %v", err), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(updatedTopic)
			}).ServeHTTP(w, r)
		} else {
			http.Error(w, "Method not allowed for restore", http.StatusMethodNotAllowed)
		}
		return
	}

	if r.Method == http.MethodGet {
		versions, err := getVersions(topicID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get versions: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]*PromptVersion{"versions": versions})
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func createTopic(name, prompt string) (*Topic, error) {
	topic := &Topic{
		ID:        uuid.New().String(),
		Name:      name,
		Prompt:    prompt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := database.DB.Exec("INSERT INTO topics (id, name, prompt, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		topic.ID, topic.Name, topic.Prompt, topic.CreatedAt, topic.UpdatedAt)
	if err != nil {
		return nil, err
	}
	err = addPromptVersion(topic.ID, topic.Prompt)
	if err != nil {
		log.Printf("Warning: Failed to create initial version: %v", err)
	}
	return topic, nil
}

func getAllTopics() ([]*Topic, error) {
	rows, err := database.DB.Query("SELECT id, name, prompt, created_at, updated_at FROM topics ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []*Topic
	for rows.Next() {
		topic := &Topic{}
		if err := rows.Scan(&topic.ID, &topic.Name, &topic.Prompt, &topic.CreatedAt, &topic.UpdatedAt); err != nil {
			return nil, err
		}
		topics = append(topics, topic)
	}
	return topics, nil
}

func getTopic(topicID string) (*Topic, error) {
	row := database.DB.QueryRow("SELECT id, name, prompt, created_at, updated_at FROM topics WHERE id = ?", topicID)
	topic := &Topic{}
	err := row.Scan(&topic.ID, &topic.Name, &topic.Prompt, &topic.CreatedAt, &topic.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return topic, nil
}

func updateTopic(topicID, name, prompt string) (*Topic, error) {
	err := addPromptVersion(topicID, prompt)
	if err != nil {
		log.Printf("Warning: Failed to create version: %v", err)
	}

	versions, err := getVersions(topicID)
	if err == nil && len(versions) > 10 {
		oldVersions := versions[:len(versions)-10]
		for _, oldVersion := range oldVersions {
			deleteVersion(oldVersion.ID)
		}
	}

	_, err = database.DB.Exec("UPDATE topics SET name = ?, prompt = ?, updated_at = ? WHERE id = ?", name, prompt, time.Now(), topicID)
	if err != nil {
		return nil, err
	}
	return getTopic(topicID)
}

func deleteTopic(topicID string) error {
	_, err := database.DB.Exec("DELETE FROM topics WHERE id = ?", topicID)
	return err
}

func getVersions(topicID string) ([]*PromptVersion, error) {
	rows, err := database.DB.Query("SELECT id, topic_id, prompt, version, created_at FROM prompt_versions WHERE topic_id = ? ORDER BY version", topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*PromptVersion
	for rows.Next() {
		version := &PromptVersion{}
		if err := rows.Scan(&version.ID, &version.TopicID, &version.Prompt, &version.Version, &version.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func getVersion(versionID string) (*PromptVersion, error) {
	row := database.DB.QueryRow("SELECT id, topic_id, prompt, version, created_at FROM prompt_versions WHERE id = ?", versionID)
	version := &PromptVersion{}
	err := row.Scan(&version.ID, &version.TopicID, &version.Prompt, &version.Version, &version.CreatedAt)
	if err != nil {
		return nil, err
	}
	return version, nil
}

func addPromptVersion(topicID, prompt string) error {
	versions, err := getVersions(topicID)
	if err != nil {
		return err
	}

	nextVersion := 1
	if len(versions) > 0 {
		nextVersion = versions[len(versions)-1].Version + 1
	}

	_, err = database.DB.Exec("INSERT INTO prompt_versions (id, topic_id, prompt, version, created_at) VALUES (?, ?, ?, ?, ?)",
		uuid.New().String(), topicID, prompt, nextVersion, time.Now())
	return err
}

func deleteVersion(versionID string) error {
	_, err := database.DB.Exec("DELETE FROM prompt_versions WHERE id = ?", versionID)
	return err
}

func getPromptHash(prompt string) string {
	hash := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(hash[:])
}

func createExercise(topicID, promptHash, exerciseJSON string) (*Exercise, error) {
	exercise := &Exercise{
		ID:           uuid.New().String(),
		TopicID:      topicID,
		PromptHash:   promptHash,
		ExerciseJSON: exerciseJSON,
	}
	_, err := database.DB.Exec("INSERT INTO exercises (id, topic_id, prompt_hash, exercise_json, created_at) VALUES (?, ?, ?, ?, ?)",
		exercise.ID, exercise.TopicID, exercise.PromptHash, exercise.ExerciseJSON, time.Now())
	if err != nil {
		return nil, err
	}
	return exercise, nil
}

func getExercisesForTopic(topicID, promptHash string) ([]*Exercise, error) {
	rows, err := database.DB.Query("SELECT id, topic_id, prompt_hash, exercise_json FROM exercises WHERE topic_id = ? AND prompt_hash = ?", topicID, promptHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exercises []*Exercise
	for rows.Next() {
		exercise := &Exercise{}
		if err := rows.Scan(&exercise.ID, &exercise.TopicID, &exercise.PromptHash, &exercise.ExerciseJSON); err != nil {
			return nil, err
		}
		exercises = append(exercises, exercise)
	}
	return exercises, nil
}

func getUserExerciseViews(userID string) (map[string]*UserExerciseView, error) {
	rows, err := database.DB.Query("SELECT id, user_id, exercise_id, last_viewed, repetition_counter FROM user_exercise_views WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	views := make(map[string]*UserExerciseView)
	for rows.Next() {
		view := &UserExerciseView{}
		if err := rows.Scan(&view.ID, &view.UserID, &view.ExerciseID, &view.LastViewed, &view.RepetitionCounter); err != nil {
			return nil, err
		}
		views[view.ExerciseID] = view
	}
	return views, nil
}

func updateUserExerciseViews(viewsToUpdate []*UserExerciseView) error {
	tx, err := database.DB.Begin()
	if err != nil {
		return err
	}

	for _, view := range viewsToUpdate {
		var count int
		tx.QueryRow("SELECT COUNT(*) FROM user_exercise_views WHERE id = ?", view.ID).Scan(&count)
		if count > 0 {
			_, err = tx.Exec("UPDATE user_exercise_views SET last_viewed = ?, repetition_counter = ? WHERE id = ?",
				view.LastViewed, view.RepetitionCounter, view.ID)
		} else {
			_, err = tx.Exec("INSERT INTO user_exercise_views (id, user_id, exercise_id, last_viewed, repetition_counter) VALUES (?, ?, ?, ?, ?)",
				view.ID, view.UserID, view.ExerciseID, view.LastViewed, view.RepetitionCounter)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}
