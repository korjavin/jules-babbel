package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GenerateRequest struct {
	TopicID string `json:"topic_id"`
}

type Topic struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Prompt      string    `json:"prompt"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PromptVersion struct {
	ID        string    `json:"id"`
	TopicID   string    `json:"topic_id"`
	Prompt    string    `json:"prompt"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}

type TopicRequest struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type UpdatePromptRequest struct {
	Prompt string `json:"prompt"`
}

type OpenAIRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	ResponseFormat struct {
		Type string `json:"type"`
	} `json:"response_format"`
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

// In-memory storage with mutex for thread safety
var (
	topics         = make(map[string]*Topic)
	promptVersions = make(map[string][]*PromptVersion) // map[topicID][]versions
	topicsMutex    sync.RWMutex
	nextID         = 1
)

func generateID() string {
	id := fmt.Sprintf("topic_%d_%d", nextID, time.Now().UnixNano())
	nextID++
	return id
}

func generateVersionID() string {
	return fmt.Sprintf("version_%d_%d", nextID, time.Now().UnixNano())
}

// Initialize with default topics
func initializeDefaultTopics() {
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

	for _, defaultTopic := range defaultTopics {
		createTopic(defaultTopic.name, defaultTopic.prompt)
	}
}

func createTopic(name, prompt string) *Topic {
	topicsMutex.Lock()
	defer topicsMutex.Unlock()

	topic := &Topic{
		ID:        generateID(),
		Name:      name,
		Prompt:    prompt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	topics[topic.ID] = topic
	
	// Create initial version
	addPromptVersion(topic.ID, prompt)
	
	return topic
}

func addPromptVersion(topicID, prompt string) {
	if promptVersions[topicID] == nil {
		promptVersions[topicID] = make([]*PromptVersion, 0)
	}

	version := &PromptVersion{
		ID:        generateVersionID(),
		TopicID:   topicID,
		Prompt:    prompt,
		Version:   len(promptVersions[topicID]) + 1,
		CreatedAt: time.Now(),
	}

	promptVersions[topicID] = append(promptVersions[topicID], version)
	
	// Keep only last 10 versions
	if len(promptVersions[topicID]) > 10 {
		promptVersions[topicID] = promptVersions[topicID][len(promptVersions[topicID])-10:]
		// Renumber versions
		for i, v := range promptVersions[topicID] {
			v.Version = i + 1
		}
	}
}

func main() {
	// Initialize default topics
	initializeDefaultTopics()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Custom handler for index.html with cache-busting
	http.HandleFunc("/", handleIndex)
	
	// Serve static files with cache headers
	http.HandleFunc("/app.js", handleJS)
	
	// API endpoints
	http.HandleFunc("/api/generate", handleGenerate)
	http.HandleFunc("/api/topics", handleTopics)
	http.HandleFunc("/api/topics/", handleTopicByID)
	http.HandleFunc("/api/versions/", handleVersions)
	
	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	// Read the HTML file
	content, err := os.ReadFile("index.html")
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
	content, err := os.ReadFile("app.js")
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	
	// Set headers for JS file - allow caching but with versioning
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year
	
	w.Write(content)
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
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
	topicsMutex.RLock()
	topic, exists := topics[req.TopicID]
	topicsMutex.RUnlock()
	
	if !exists {
		http.Error(w, "Topic not found", http.StatusNotFound)
		return
	}

	// Create OpenAI request
	openaiReq := OpenAIRequest{
		Model: modelName,
		Messages: []Message{
			{
				Role:    "user",
				Content: topic.Prompt,
			},
		},
	}
	openaiReq.ResponseFormat.Type = "json_object"

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
		errorResp := map[string]interface{}{
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

// Handle topics CRUD operations
func handleTopics(w http.ResponseWriter, r *http.Request) {
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
		topicsMutex.RLock()
		topicsList := make([]*Topic, 0, len(topics))
		for _, topic := range topics {
			topicsList = append(topicsList, topic)
		}
		topicsMutex.RUnlock()
		
		// Sort by creation time
		sort.Slice(topicsList, func(i, j int) bool {
			return topicsList[i].CreatedAt.Before(topicsList[j].CreatedAt)
		})
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]*Topic{"topics": topicsList})

	case http.MethodPost:
		var req TopicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		
		if req.Name == "" || req.Prompt == "" {
			http.Error(w, "Name and prompt are required", http.StatusBadRequest)
			return
		}
		
		topic := createTopic(req.Name, req.Prompt)
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(topic)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Handle individual topic operations
func handleTopicByID(w http.ResponseWriter, r *http.Request) {
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
		topicsMutex.RLock()
		topic, exists := topics[topicID]
		topicsMutex.RUnlock()
		
		if !exists {
			http.Error(w, "Topic not found", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(topic)

	case http.MethodPut:
		var req UpdatePromptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		
		if req.Prompt == "" {
			http.Error(w, "Prompt is required", http.StatusBadRequest)
			return
		}
		
		topicsMutex.Lock()
		topic, exists := topics[topicID]
		if !exists {
			topicsMutex.Unlock()
			http.Error(w, "Topic not found", http.StatusNotFound)
			return
		}
		
		// Add new version before updating
		addPromptVersion(topicID, req.Prompt)
		
		// Update topic
		topic.Prompt = req.Prompt
		topic.UpdatedAt = time.Now()
		topicsMutex.Unlock()
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(topic)

	case http.MethodDelete:
		topicsMutex.Lock()
		_, exists := topics[topicID]
		if !exists {
			topicsMutex.Unlock()
			http.Error(w, "Topic not found", http.StatusNotFound)
			return
		}
		
		delete(topics, topicID)
		delete(promptVersions, topicID)
		topicsMutex.Unlock()
		
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Handle prompt versions
func handleVersions(w http.ResponseWriter, r *http.Request) {
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
		topicsMutex.RLock()
		versions := promptVersions[topicID]
		topicsMutex.RUnlock()
		
		if versions == nil {
			http.Error(w, "Topic not found", http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]*PromptVersion{"versions": versions})

	case http.MethodPost:
		// Restore version: POST /api/versions/{topicID}/restore/{versionID}
		if len(pathParts) < 3 || pathParts[1] != "restore" {
			http.Error(w, "Invalid restore path", http.StatusBadRequest)
			return
		}
		
		versionID := pathParts[2]
		
		topicsMutex.Lock()
		topic, topicExists := topics[topicID]
		versions := promptVersions[topicID]
		
		if !topicExists || versions == nil {
			topicsMutex.Unlock()
			http.Error(w, "Topic not found", http.StatusNotFound)
			return
		}
		
		var versionToRestore *PromptVersion
		for _, v := range versions {
			if v.ID == versionID {
				versionToRestore = v
				break
			}
		}
		
		if versionToRestore == nil {
			topicsMutex.Unlock()
			http.Error(w, "Version not found", http.StatusNotFound)
			return
		}
		
		// Create new version with restored content
		addPromptVersion(topicID, versionToRestore.Prompt)
		
		// Update topic
		topic.Prompt = versionToRestore.Prompt
		topic.UpdatedAt = time.Now()
		topicsMutex.Unlock()
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(topic)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}