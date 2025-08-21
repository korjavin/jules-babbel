package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type GenerateRequest struct {
	MasterPrompt string `json:"master_prompt"`
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

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Custom handler for index.html with cache-busting
	http.HandleFunc("/", handleIndex)
	
	// Serve static files with cache headers
	http.HandleFunc("/app.js", handleJS)
	
	// API endpoint for generating exercises
	http.HandleFunc("/api/generate", handleGenerate)
	
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

	// Create OpenAI request
	openaiReq := OpenAIRequest{
		Model: modelName,
		Messages: []Message{
			{
				Role:    "user",
				Content: req.MasterPrompt,
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