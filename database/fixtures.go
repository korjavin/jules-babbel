package database

import (
	"log"
	"time"

	"github.com/google/uuid"
)

func LoadFixtures() {
	var count int
	DB.QueryRow("SELECT COUNT(*) FROM topics").Scan(&count)
	if count > 0 {
		log.Println("Topics table already has data, skipping fixtures.")
		return
	}

	log.Println("Loading fixtures...")

	defaultTopics := []struct {
		name   string
		prompt string
	}{
		{
			name: "Conjunctions",
			prompt: `You are an expert German language tutor creating B1-level grammar exercises. Your task is to generate a JSON object containing unique sentences focused on German conjunctions.\n\nPlease adhere to the following rules:\n1. **Sentence Structure:** Each sentence must correctly use a German conjunction. Include a mix of coordinating and subordinating conjunctions from the provided list.\n2. **Vocabulary:** Use common B1-level vocabulary.\n3. **Clarity:** The English hint must be a natural and accurate translation of the German sentence.\nConjunction List: weil, obwohl, damit, wenn, dass, als, bevor, nachdem, ob, seit, und, oder, aber, denn, sondern.\n\nReturn ONLY the JSON object, with no other text or explanations.`,
		},
		{
			name: "Verb + Preposition",
			prompt: `You are an expert German language tutor creating B1-level exercises focused on German verbs with prepositions. Your task is to generate a JSON object containing unique sentences that practice verb-preposition combinations.\n\nPlease adhere to the following rules:\n1. **Sentence Structure:** Each sentence must correctly use a German verb with its required preposition.\n2. **Vocabulary:** Use common B1-level vocabulary.\n3. **Clarity:** The English hint must be a natural and accurate translation of the German sentence.\nCommon verb-preposition combinations: denken an, warten auf, sich freuen über, sprechen über, bitten um, sich interessieren für, etc.\n\nReturn ONLY the JSON object, with no other text or explanations.`,
		},
		{
			name: "Preterite vs Perfect",
			prompt: `You are an expert German language tutor creating B1-level exercises focused on the correct usage of Preterite (Präteritum) vs Perfect tense (Perfekt) in German. Your task is to generate a JSON object containing unique sentences that practice these tenses.\n\nPlease adhere to the following rules:\n1. **Sentence Structure:** Each sentence must demonstrate the appropriate use of either Preterite or Perfect tense.\n2. **Vocabulary:** Use common B1-level vocabulary.\n3. **Clarity:** The English hint must be a natural and accurate translation of the German sentence.\nFocus on: written vs spoken contexts, completed actions, narrative vs conversational style.\n\nReturn ONLY the JSON object, with no other text or explanations.`,
		},
	}

	for _, t := range defaultTopics {
		topicID := uuid.New().String()
		now := time.Now()
		_, err := DB.Exec("INSERT INTO topics (id, name, prompt, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			topicID, t.name, t.prompt, now, now)
		if err != nil {
			log.Printf("Failed to insert topic %s: %v", t.name, err)
		}
	}

	log.Println("Fixtures loaded.")
}
