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

	"github.com/mehanizm/airtable"
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

// Airtable configuration
var (
	airtableClient   *airtable.Client
	airtableBaseID   string
	topicsMutex      sync.RWMutex
	
	// Table names
	topicsTableName   = "Topics"
	versionsTableName = "PromptVersions"

	// For observability
	lastRefinedPrompt      string
	lastRefinedPromptMutex sync.RWMutex
)

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


// Initialize Airtable client
func initStorage() {
	airtableToken := os.Getenv("AIRTABLE_TOKEN")
	airtableBaseID = os.Getenv("AIRTABLE_BASE_ID")
	
	if airtableToken == "" {
		log.Fatal("AIRTABLE_TOKEN environment variable is required")
	}
	if airtableBaseID == "" {
		log.Fatal("AIRTABLE_BASE_ID environment variable is required")
	}
	
	airtableClient = airtable.NewClient(airtableToken)
	log.Printf("Airtable integration initialized with base ID: %s", airtableBaseID)
	
	// Verify and setup tables
	err := setupAirtableTables()
	if err != nil {
		log.Printf("Warning: Could not setup Airtable tables: %v", err)
	}
	
	// Check permissions
	checkAirtablePermissions()
}

// Setup Airtable tables if they don't exist or verify their structure
func setupAirtableTables() error {
	log.Printf("Setting up Airtable tables...")
	
	// Try to create the tables using Airtable's API
	err := createAirtableTables()
	if err != nil {
		log.Printf("Could not auto-create tables: %v", err)
		return err
	}
	
	return nil
}

// Create Airtable tables using the Metadata API
func createAirtableTables() error {
	// Note: Airtable's table creation via API requires Base Schema API access
	// For now, we'll provide instructions for manual creation
	
	log.Printf("Please manually create these tables in your Airtable base:")
	log.Printf("")
	log.Printf("📋 Table 1: 'Topics'")
	log.Printf("   • Name: Single line text")
	log.Printf("   • Prompt: Long text")
	log.Printf("   • CreatedAt: Single line text (optional)")
	log.Printf("   • UpdatedAt: Single line text (optional)")
	log.Printf("")
	log.Printf("📋 Table 2: 'PromptVersions'") 
	log.Printf("   • TopicID: Single line text")
	log.Printf("   • Prompt: Long text")
	log.Printf("   • Version: Number")
	log.Printf("   • CreatedAt: Single line text (optional)")
	log.Printf("")
	log.Printf("💡 Tip: The timestamp fields (CreatedAt, UpdatedAt) are optional.")
	log.Printf("💡 The app will work with just the required fields if timestamps are missing.")
	log.Printf("")
	
	return fmt.Errorf("manual table creation required")
}

// Check Airtable permissions for both tables
func checkAirtablePermissions() {
	log.Printf("Checking Airtable permissions...")
	
	// Test Topics table access
	topicsTable := airtableClient.GetTable(airtableBaseID, topicsTableName)
	_, err := topicsTable.GetRecords().Do()
	if err != nil {
		if strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "INVALID_PERMISSIONS") {
			log.Printf("❌ No access to 'Topics' table. Check token permissions.")
		} else if strings.Contains(err.Error(), "status 404") {
			log.Printf("❌ 'Topics' table not found. Please create it manually.")
		} else {
			log.Printf("⚠️  Topics table access error: %v", err)
		}
	} else {
		log.Printf("✅ Topics table access: OK")
	}
	
	// Test PromptVersions table access
	versionsTable := airtableClient.GetTable(airtableBaseID, versionsTableName)
	_, err = versionsTable.GetRecords().Do()
	if err != nil {
		if strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "INVALID_PERMISSIONS") {
			log.Printf("❌ No access to 'PromptVersions' table. Check token permissions.")
			log.Printf("💡 Version history will be disabled but core functionality will work.")
		} else if strings.Contains(err.Error(), "status 404") {
			log.Printf("❌ 'PromptVersions' table not found. Please create it manually.")
			log.Printf("💡 Version history will be disabled but core functionality will work.")
		} else {
			log.Printf("⚠️  PromptVersions table access error: %v", err)
		}
	} else {
		log.Printf("✅ PromptVersions table access: OK")
	}
}

// Initialize with default topics
func initializeDefaultTopics() {
	// Check if we already have topics (to avoid duplicating on restart)
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

// Data access functions using Airtable
func createTopic(name, prompt string) (*Topic, error) {
	table := airtableClient.GetTable(airtableBaseID, topicsTableName)
	now := time.Now().Format(time.RFC3339)
	
	// Try with timestamp fields first, fallback to just required fields
	fields := map[string]any{
		"Name":   name,
		"Prompt": prompt,
	}
	
	// Try to add timestamp fields if they exist
	records := &airtable.Records{
		Records: []*airtable.Record{
			{
				Fields: map[string]any{
					"Name":      name,
					"Prompt":    prompt,
					"CreatedAt": now,
					"UpdatedAt": now,
				},
			},
		},
	}
	
	result, err := table.AddRecords(records)
	if err != nil {
		// If it failed due to unknown fields, try with minimal fields
		if strings.Contains(err.Error(), "UNKNOWN_FIELD_NAME") {
			log.Printf("Timestamp fields not found, creating with minimal fields")
			records.Records[0].Fields = fields
			result, err = table.AddRecords(records)
		}
		
		if err != nil {
			return nil, fmt.Errorf("failed to create topic in Airtable: %v", err)
		}
	}
	
	if len(result.Records) == 0 {
		return nil, fmt.Errorf("no records returned from Airtable")
	}
	
	topic := &Topic{
		ID:        result.Records[0].ID,
		Name:      name,
		Prompt:    prompt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	// Create initial version
	err = addPromptVersion(topic.ID, prompt)
	if err != nil {
		log.Printf("Warning: Failed to create initial version: %v", err)
	}
	
	return topic, nil
}

func getAllTopics() ([]*Topic, error) {
	table := airtableClient.GetTable(airtableBaseID, topicsTableName)
	
	records, err := table.GetRecords().Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get topics from Airtable: %v", err)
	}
	
	var topics []*Topic
	for _, record := range records.Records {
		topic := &Topic{
			ID: record.ID,
		}
		
		if name, ok := record.Fields["Name"].(string); ok {
			topic.Name = name
		}
		if prompt, ok := record.Fields["Prompt"].(string); ok {
			topic.Prompt = prompt
		}
		if createdAt, ok := record.Fields["CreatedAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
				topic.CreatedAt = t
			}
		}
		if updatedAt, ok := record.Fields["UpdatedAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
				topic.UpdatedAt = t
			}
		}
		
		topics = append(topics, topic)
	}
	
	// Sort by creation time
	sort.Slice(topics, func(i, j int) bool {
		return topics[i].CreatedAt.Before(topics[j].CreatedAt)
	})
	
	return topics, nil
}

func getTopic(topicID string) (*Topic, error) {
	table := airtableClient.GetTable(airtableBaseID, topicsTableName)
	
	record, err := table.GetRecord(topicID)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic from Airtable: %v", err)
	}
	
	topic := &Topic{
		ID: record.ID,
	}
	
	if name, ok := record.Fields["Name"].(string); ok {
		topic.Name = name
	}
	if prompt, ok := record.Fields["Prompt"].(string); ok {
		topic.Prompt = prompt
	}
	if createdAt, ok := record.Fields["CreatedAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			topic.CreatedAt = t
		}
	}
	if updatedAt, ok := record.Fields["UpdatedAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			topic.UpdatedAt = t
		}
	}
	
	return topic, nil
}

func updateTopic(topicID, prompt string) (*Topic, error) {
	table := airtableClient.GetTable(airtableBaseID, topicsTableName)
	now := time.Now().Format(time.RFC3339)
	
	// First add the new version
	err := addPromptVersion(topicID, prompt)
	if err != nil {
		log.Printf("Warning: Failed to create version: %v", err)
	}
	
	// Clean up old versions (keep only last 10)
	versions, err := getVersions(topicID)
	if err == nil && len(versions) > 10 {
		versionsTable := airtableClient.GetTable(airtableBaseID, versionsTableName)
		oldVersions := versions[:len(versions)-10] // Keep last 10
		var oldVersionIDs []string
		for _, oldVersion := range oldVersions {
			oldVersionIDs = append(oldVersionIDs, oldVersion.ID)
		}
		versionsTable.DeleteRecords(oldVersionIDs)
	}
	
	// Update the topic
	records := &airtable.Records{
		Records: []*airtable.Record{
			{
				ID: topicID,
				Fields: map[string]any{
					"Prompt":    prompt,
					"UpdatedAt": now,
				},
			},
		},
	}
	
	_, err = table.UpdateRecords(records)
	if err != nil {
		// If UpdatedAt field doesn't exist, try without it
		if strings.Contains(err.Error(), "UNKNOWN_FIELD_NAME") {
			log.Printf("UpdatedAt field not found, updating with minimal fields")
			records.Records[0].Fields = map[string]any{
				"Prompt": prompt,
			}
			_, err = table.UpdateRecords(records)
		}
		
		if err != nil {
			return nil, fmt.Errorf("failed to update topic in Airtable: %v", err)
		}
	}
	
	return getTopic(topicID)
}

func deleteTopic(topicID string) error {
	// First delete all versions for this topic
	versions, err := getVersions(topicID)
	if err == nil && len(versions) > 0 {
		versionsTable := airtableClient.GetTable(airtableBaseID, versionsTableName)
		var versionIDs []string
		for _, version := range versions {
			versionIDs = append(versionIDs, version.ID)
		}
		versionsTable.DeleteRecords(versionIDs)
	}
	
	// Then delete the topic
	table := airtableClient.GetTable(airtableBaseID, topicsTableName)
	_, err = table.DeleteRecords([]string{topicID})
	if err != nil {
		return fmt.Errorf("failed to delete topic from Airtable: %v", err)
	}
	
	return nil
}

func getVersions(topicID string) ([]*PromptVersion, error) {
	table := airtableClient.GetTable(airtableBaseID, versionsTableName)
	
	records, err := table.GetRecords().
		WithFilterFormula(fmt.Sprintf("{TopicID} = '%s'", topicID)).
		Do()
	
	if err != nil {
		// Check for permission errors
		if strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "INVALID_PERMISSIONS") {
			log.Printf("No read access to PromptVersions table. Version history unavailable.")
			return []*PromptVersion{}, nil // Return empty slice instead of error
		}
		return nil, fmt.Errorf("failed to get versions from Airtable: %v", err)
	}
	
	var versions []*PromptVersion
	for _, record := range records.Records {
		version := &PromptVersion{
			ID: record.ID,
		}
		
		if topicIDField, ok := record.Fields["TopicID"].(string); ok {
			version.TopicID = topicIDField
		}
		if prompt, ok := record.Fields["Prompt"].(string); ok {
			version.Prompt = prompt
		}
		if versionNum, ok := record.Fields["Version"].(float64); ok {
			version.Version = int(versionNum)
		}
		if createdAt, ok := record.Fields["CreatedAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
				version.CreatedAt = t
			}
		}
		
		versions = append(versions, version)
	}
	
	// Sort by version number
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})
	
	return versions, nil
}

func getVersion(versionID string) (*PromptVersion, error) {
	table := airtableClient.GetTable(airtableBaseID, versionsTableName)
	
	record, err := table.GetRecord(versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get version from Airtable: %v", err)
	}
	
	version := &PromptVersion{
		ID: record.ID,
	}
	
	if topicID, ok := record.Fields["TopicID"].(string); ok {
		version.TopicID = topicID
	}
	if prompt, ok := record.Fields["Prompt"].(string); ok {
		version.Prompt = prompt
	}
	if versionNum, ok := record.Fields["Version"].(float64); ok {
		version.Version = int(versionNum)
	}
	if createdAt, ok := record.Fields["CreatedAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			version.CreatedAt = t
		}
	}
	
	return version, nil
}

func addPromptVersion(topicID, prompt string) error {
	// Get existing versions to determine next version number
	versions, err := getVersions(topicID)
	if err != nil {
		// Check if it's a permission error
		if strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "INVALID_PERMISSIONS") {
			log.Printf("No access to PromptVersions table. Please check Airtable token permissions.")
			log.Printf("The token needs read/write access to both 'Topics' and 'PromptVersions' tables.")
			return nil // Don't fail the topic creation due to version permission issues
		}
		if !strings.Contains(err.Error(), "status 404") {
			return err
		}
	}
	
	nextVersion := 1
	if len(versions) > 0 {
		nextVersion = versions[len(versions)-1].Version + 1
	}
	
	table := airtableClient.GetTable(airtableBaseID, versionsTableName)
	now := time.Now().Format(time.RFC3339)
	
	// Try with timestamp field first, fallback to minimal fields
	records := &airtable.Records{
		Records: []*airtable.Record{
			{
				Fields: map[string]any{
					"TopicID":   topicID,
					"Prompt":    prompt,
					"Version":   nextVersion,
					"CreatedAt": now,
				},
			},
		},
	}
	
	_, err = table.AddRecords(records)
	if err != nil {
		// Check for permission errors
		if strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "INVALID_PERMISSIONS") {
			log.Printf("No write access to PromptVersions table. Skipping version creation.")
			return nil // Don't fail the topic creation
		}
		
		// If it failed due to unknown fields, try with minimal fields
		if strings.Contains(err.Error(), "UNKNOWN_FIELD_NAME") {
			log.Printf("CreatedAt field not found in PromptVersions, creating with minimal fields")
			records.Records[0].Fields = map[string]any{
				"TopicID": topicID,
				"Prompt":  prompt,
				"Version": nextVersion,
			}
			_, err = table.AddRecords(records)
		}
		
		if err != nil {
			// Final check for permissions before failing
			if strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "INVALID_PERMISSIONS") {
				log.Printf("Cannot create version due to permissions. Continuing without version tracking.")
				return nil
			}
			return fmt.Errorf("failed to create version in Airtable: %v", err)
		}
	}
	
	return nil
}

func main() {
	// Initialize storage backend
	initStorage()
	
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
	http.HandleFunc("/api/last-refined-prompt", handleGetLastRefinedPrompt)
	
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
		topic, err := getTopic(topicID)
		if err != nil {
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
		
		topic, err := updateTopic(topicID, req.Prompt)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to update topic: %v", err), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(topic)

	case http.MethodDelete:
		err := deleteTopic(topicID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete topic: %v", err), http.StatusInternalServerError)
			return
		}
		
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
		versions, err := getVersions(topicID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get versions: %v", err), http.StatusInternalServerError)
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
		
		// Update topic with restored prompt (this will automatically create a new version)
		topic, err := updateTopic(topicID, versionToRestore.Prompt)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to restore version: %v", err), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(topic)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}