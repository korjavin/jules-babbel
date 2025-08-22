package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mehanizm/airtable"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	oauth2v2 "google.golang.org/api/oauth2/v2"
	"golang.org/x/time/rate"
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

type Exercise struct {
	ID           string    `json:"id"`
	AirtableID   string    `json:"airtable_id"`
	TopicID      string    `json:"topic_id"`
	PromptHash   string    `json:"prompt_hash"`
	ExerciseJSON string    `json:"exercise_json"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserExerciseView struct {
	ID                string    `json:"id"`
	AirtableID        string    `json:"airtable_id"`
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
	ID         string `json:"id"`
	GoogleID   string `json:"google_id"`
	AirtableID string `json:"airtable_id"`
}

type UserStats struct {
	UserID             string `json:"user_id"`
	TotalExercises     int    `json:"total_exercises"`
	TotalMistakes      int    `json:"total_mistakes"`
	TotalHints         int    `json:"total_hints"`
	TotalTime          int    `json:"total_time"`
	LastTopicID        string `json:"last_topic_id"`
	AirtableRecordID   string `json:"airtable_record_id"`
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

// Airtable configuration
var (
	airtableClient   *airtable.Client
	airtableBaseID   string
	topicsMutex      sync.RWMutex
	
	// Table names
	topicsTableName            = "Topics"
	versionsTableName          = "PromptVersions"
	usersTableName             = "Users"
	userStatsTableName         = "UserStats"
	exercisesTableName         = "Exercises"
	userExerciseViewsTableName = "UserExerciseViews"

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
	log.Printf("📋 Table 3: 'Exercises'")
	log.Printf("   • TopicID: Single line text (Link to 'Topics' table is recommended)")
	log.Printf("   • PromptHash: Single line text")
	log.Printf("   • ExerciseJSON: Long text")
	log.Printf("   • CreatedAt: Created time (Airtable managed)")
	log.Printf("")
	log.Printf("📋 Table 4: 'UserExerciseViews'")
	log.Printf("   • UserID: Single line text (Link to 'Users' table is recommended)")
	log.Printf("   • ExerciseID: Single line text (Link to 'Exercises' table is recommended)")
	log.Printf("   • LastViewed: Date and time")
	log.Printf("   • RepetitionCounter: Number (Default to 0)")
	log.Printf("   • NextReview: Formula (Optional, for debugging). Formula: DATEADD({LastViewed}, POWER({RepetitionCounter}, 2), 'days')")
	log.Printf("")
	log.Printf("💡 Tip: The timestamp fields (CreatedAt, UpdatedAt) are optional.")
	log.Printf("💡 The app will work with just the required fields if timestamps are missing.")
	log.Printf("")

	return fmt.Errorf("manual table creation required")
}

// Check Airtable permissions for all tables
func checkAirtablePermissions() {
	log.Printf("Checking Airtable permissions...")

	tables := []struct {
		name        string
		required    bool
		description string
	}{
		{topicsTableName, true, "Core functionality will be severely limited."},
		{versionsTableName, false, "Version history will be disabled."},
		{usersTableName, false, "User authentication will be disabled."},
		{userStatsTableName, false, "User statistics will not be saved."},
		{exercisesTableName, true, "Core functionality of serving exercises will be disabled."},
		{userExerciseViewsTableName, false, "SRS functionality will be disabled for authenticated users."},
	}

	for _, table := range tables {
		checkTableAccess(table.name, table.required, table.description)
	}
}

func checkTableAccess(tableName string, required bool, consequence string) {
	table := airtableClient.GetTable(airtableBaseID, tableName)
	_, err := table.GetRecords().Do() // Check without max records for compatibility

	if err != nil {
		prefix := "⚠️"
		if required {
			prefix = "❌"
		}

		if strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "INVALID_PERMISSIONS") {
			log.Printf("%s No access to '%s' table. Check token permissions. %s", prefix, tableName, consequence)
		} else if strings.Contains(err.Error(), "status 404") {
			log.Printf("%s '%s' table not found. Please create it manually. %s", prefix, tableName, consequence)
		} else {
			log.Printf("⚠️  %s table access error: %v", tableName, err)
		}
	} else {
		log.Printf("✅ %s table access: OK", tableName)
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

func updateTopic(topicID, name, prompt string) (*Topic, error) {
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

	// Prepare fields for update
	fields := map[string]any{
		"Prompt":    prompt,
		"UpdatedAt": now,
	}
	if name != "" {
		fields["Name"] = name
	}

	records := &airtable.Records{
		Records: []*airtable.Record{
			{
				ID:     topicID,
				Fields: fields,
			},
		},
	}

	_, err = table.UpdateRecords(records)
	if err != nil {
		if strings.Contains(err.Error(), "UNKNOWN_FIELD_NAME") {
			log.Printf("UpdatedAt field not found, updating with minimal fields")
			delete(fields, "UpdatedAt")
			records.Records[0].Fields = fields
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
			if strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "INVALID__PERMISSIONS") {
				log.Printf("Cannot create version due to permissions. Continuing without version tracking.")
				return nil
			}
			return fmt.Errorf("failed to create version in Airtable: %v", err)
		}
	}

	return nil
}

func getPromptHash(prompt string) string {
	hash := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(hash[:])
}

func createExercise(topicID, promptHash, exerciseJSON string) (*Exercise, error) {
	table := airtableClient.GetTable(airtableBaseID, exercisesTableName)
	records := &airtable.Records{
		Records: []*airtable.Record{
			{
				Fields: map[string]any{
					"TopicID":      topicID,
					"PromptHash":   promptHash,
					"ExerciseJSON": exerciseJSON,
				},
			},
		},
	}

	result, err := table.AddRecords(records)
	if err != nil {
		return nil, fmt.Errorf("failed to create exercise in Airtable: %v", err)
	}

	if len(result.Records) == 0 {
		return nil, fmt.Errorf("no records returned from Airtable")
	}

	rec := result.Records[0]
	exercise := &Exercise{
		AirtableID:   rec.ID,
		TopicID:      topicID,
		PromptHash:   promptHash,
		ExerciseJSON: exerciseJSON,
		CreatedAt:    time.Now(), // Approximate, actual time is on Airtable
	}
	return exercise, nil
}

func getExercisesForTopic(topicID, promptHash string) ([]*Exercise, error) {
	table := airtableClient.GetTable(airtableBaseID, exercisesTableName)
	formula := fmt.Sprintf("AND({TopicID} = '%s', {PromptHash} = '%s')", topicID, promptHash)

	records, err := table.GetRecords().WithFilterFormula(formula).Do()
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			return []*Exercise{}, nil // Return empty slice if table not found
		}
		return nil, fmt.Errorf("failed to get exercises from Airtable: %v", err)
	}

	var exercises []*Exercise
	for _, record := range records.Records {
		exercise := &Exercise{
			AirtableID: record.ID,
		}
		if val, ok := record.Fields["TopicID"].(string); ok {
			exercise.TopicID = val
		}
		if val, ok := record.Fields["PromptHash"].(string); ok {
			exercise.PromptHash = val
		}
		if val, ok := record.Fields["ExerciseJSON"].(string); ok {
			exercise.ExerciseJSON = val
		}
		if val, ok := record.Fields["CreatedAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				exercise.CreatedAt = t
			}
		}
		exercises = append(exercises, exercise)
	}
	return exercises, nil
}

func getUserExerciseViews(userID string) (map[string]*UserExerciseView, error) {
	table := airtableClient.GetTable(airtableBaseID, userExerciseViewsTableName)
	formula := fmt.Sprintf("{UserID} = '%s'", userID)

	records, err := table.GetRecords().WithFilterFormula(formula).Do()
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			return make(map[string]*UserExerciseView), nil // Return empty map if table not found
		}
		return nil, fmt.Errorf("failed to get user exercise views from Airtable: %v", err)
	}

	views := make(map[string]*UserExerciseView)
	for _, record := range records.Records {
		view := &UserExerciseView{
			AirtableID: record.ID,
		}
		if val, ok := record.Fields["UserID"].(string); ok {
			view.UserID = val
		}
		if val, ok := record.Fields["ExerciseID"].(string); ok {
			view.ExerciseID = val
		}
		if val, ok := record.Fields["LastViewed"].(string); ok {
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				view.LastViewed = t
			}
		}
		if val, ok := record.Fields["RepetitionCounter"].(float64); ok {
			view.RepetitionCounter = int(val)
		}
		views[view.ExerciseID] = view
	}
	return views, nil
}

func updateUserExerciseViews(viewsToUpdate []*UserExerciseView) error {
	table := airtableClient.GetTable(airtableBaseID, userExerciseViewsTableName)
	var recordsToCreate []*airtable.Record
	var recordsToUpdate []*airtable.Record

	for _, view := range viewsToUpdate {
		fields := map[string]any{
			"UserID":            view.UserID,
			"ExerciseID":        view.ExerciseID,
			"LastViewed":        view.LastViewed.Format(time.RFC3339),
			"RepetitionCounter": view.RepetitionCounter,
		}
		if view.AirtableID == "" {
			recordsToCreate = append(recordsToCreate, &airtable.Record{Fields: fields})
		} else {
			recordsToUpdate = append(recordsToUpdate, &airtable.Record{ID: view.AirtableID, Fields: fields})
		}
	}

	if len(recordsToCreate) > 0 {
		if _, err := table.AddRecords(&airtable.Records{Records: recordsToCreate}); err != nil {
			return fmt.Errorf("failed to create user exercise views: %v", err)
		}
	}
	if len(recordsToUpdate) > 0 {
		if _, err := table.UpdateRecords(&airtable.Records{Records: recordsToUpdate}); err != nil {
			return fmt.Errorf("failed to update user exercise views: %v", err)
		}
	}
	return nil
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
		// Guest user logic - only serve from cache, never generate.
		finalExercises = getRandomExercises(allExercises, 10)
	} else {
		// Authenticated user SRS logic
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

		// Update views for the selected exercises
		var viewsToUpdate []*UserExerciseView
		now := time.Now()
		for _, ex := range finalExercises {
			view, exists := userViews[ex.AirtableID]
			if !exists {
				view = &UserExerciseView{
					UserID:     userID,
					ExerciseID: ex.AirtableID,
				}
			}
			view.LastViewed = now
			view.RepetitionCounter++
			viewsToUpdate = append(viewsToUpdate, view)
		}
		if err := updateUserExerciseViews(viewsToUpdate); err != nil {
			log.Printf("Warning: failed to update user exercise views: %v", err)
			// Don't block user, just log the error
		}
	}

	// Prepare response
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
		view, seen := userViews[ex.AirtableID]
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
	table := airtableClient.GetTable(airtableBaseID, usersTableName)
	records, err := table.GetRecords().WithFilterFormula(fmt.Sprintf("{GoogleID} = '%s'", googleID)).Do()
	if err != nil {
		return nil, err
	}

	if len(records.Records) == 0 {
		return nil, nil // Not found
	}

	record := records.Records[0]
	return &User{
		ID:         record.ID,
		GoogleID:   record.Fields["GoogleID"].(string),
		AirtableID: record.ID,
	}, nil
}

func createUser(googleID string) (*User, error) {
	table := airtableClient.GetTable(airtableBaseID, usersTableName)
	records := &airtable.Records{
		Records: []*airtable.Record{
			{
				Fields: map[string]any{
					"GoogleID": googleID,
				},
			},
		},
	}
	result, err := table.AddRecords(records)
	if err != nil {
		return nil, err
	}

	record := result.Records[0]
	return &User{
		ID:         record.ID,
		GoogleID:   record.Fields["GoogleID"].(string),
		AirtableID: record.ID,
	}, nil
}

func getUserStats(userID string) (*UserStats, error) {
	table := airtableClient.GetTable(airtableBaseID, userStatsTableName)
	records, err := table.GetRecords().WithFilterFormula(fmt.Sprintf("{UserID} = '%s'", userID)).Do()
	if err != nil {
		return nil, err
	}

	if len(records.Records) == 0 {
		return &UserStats{UserID: userID}, nil // Return empty stats if not found
	}

	record := records.Records[0]
	stats := &UserStats{
		UserID:           userID,
		AirtableRecordID: record.ID,
	}

	if val, ok := record.Fields["TotalExercises"].(float64); ok {
		stats.TotalExercises = int(val)
	}
	if val, ok := record.Fields["TotalMistakes"].(float64); ok {
		stats.TotalMistakes = int(val)
	}
	if val, ok := record.Fields["TotalHints"].(float64); ok {
		stats.TotalHints = int(val)
	}
	if val, ok := record.Fields["TotalTime"].(float64); ok {
		stats.TotalTime = int(val)
	}
	if val, ok := record.Fields["LastTopicID"].(string); ok {
		stats.LastTopicID = val
	}

	return stats, nil
}

func updateUserStats(stats *UserStats) error {
	table := airtableClient.GetTable(airtableBaseID, userStatsTableName)
	fields := map[string]any{
		"UserID":         stats.UserID,
		"TotalExercises": stats.TotalExercises,
		"TotalMistakes":  stats.TotalMistakes,
		"TotalHints":     stats.TotalHints,
		"TotalTime":      stats.TotalTime,
	}

	if stats.AirtableRecordID != "" {
		// Update existing record
		records := &airtable.Records{
			Records: []*airtable.Record{
				{
					ID:     stats.AirtableRecordID,
					Fields: fields,
				},
			},
		}
		_, err := table.UpdateRecords(records)
		return err
	}

	// Create new record
	records := &airtable.Records{
		Records: []*airtable.Record{
			{
				Fields: fields,
			},
		},
	}
	_, err := table.AddRecords(records)
	return err
}

func updateUserSetting(userID, lastTopicID string) error {
	stats, err := getUserStats(userID)
	if err != nil {
		return err
	}

	stats.LastTopicID = lastTopicID

	table := airtableClient.GetTable(airtableBaseID, userStatsTableName)
	fields := map[string]any{
		"UserID":      userID,
		"LastTopicID": lastTopicID,
	}

	if stats.AirtableRecordID != "" {
		// Update existing record
		records := &airtable.Records{
			Records: []*airtable.Record{
				{
					ID:     stats.AirtableRecordID,
					Fields: fields,
				},
			},
		}
		_, err := table.UpdateRecords(records)
		return err
	}

	// Create new record
	records := &airtable.Records{
		Records: []*airtable.Record{
			{
				Fields: fields,
			},
		},
	}
	_, err = table.AddRecords(records)
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

	// Get or create user in Airtable
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
		Expires:  time.Now().Add(30 * 24 * time.Hour), // 30 days
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
	table := airtableClient.GetTable(airtableBaseID, usersTableName)
	record, err := table.GetRecord(userID)
	if err != nil {
		return nil, err
	}

	if record == nil {
		return nil, nil // Not found
	}

	return &User{
		ID:         record.ID,
		GoogleID:   record.Fields["GoogleID"].(string),
		AirtableID: record.ID,
	}, nil
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