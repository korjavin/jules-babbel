# German Conjunctions Trainer - Agent Documentation

## Project Overview
A web-based application for learning German grammar. It features interactive word-scramble exercises, customizable topics, and a unique prompt refinement system to ensure high-quality, varied content. The application tracks user performance and provides session statistics.

## Architecture
- **Backend**: Go HTTP server featuring an AI-powered prompt refinement system, API proxying to OpenAI, and Airtable integration for data persistence.
- **Frontend**: Vanilla JavaScript with Tailwind CSS, providing a responsive UI for exercises and topics management.
- **Storage**: Airtable for storing grammar topics and their version history.
- **Deployment**: Docker container with environment-based configuration for easy deployment.

## File Structure
```
.
├── main.go              # Go backend server with API and Airtable integration
├── index.html           # Main application UI
├── app.js               # Frontend JavaScript for interactivity and topics management
├── agent.md             # Context file for AI development
├── Dockerfile           # Container definition for production
├── docker-compose.yml   # Docker Compose for local development
├── go.mod               # Go module dependencies
├── example.prompt.md    # Example prompt for exercise generation
└── .github/workflows/   # CI/CD pipelines for Docker builds
```

## Backend (main.go)
### Key Components:
- **Prompt Refinement**: The `refinePrompt` function uses a `metaPrompt` to instruct an LLM to improve user-provided prompts, enhancing exercise quality.
- **Static File Serving**: Custom handlers serve `index.html` with dynamic cache-busting and `app.js` with long-term caching.
- **API Proxy**: The `/api/generate` endpoint securely proxies requests to the OpenAI API, using the refined prompt.
- **Rate Limiting**: IP-based rate limiting (1 request every 3 seconds) to prevent abuse.
- **Airtable Integration**: Manages CRUD operations for topics and prompt versions.

### Environment Variables:
- `OPENAI_API_KEY`: Required for AI exercise generation.
- `AIRTABLE_TOKEN`: Required for Airtable integration.
- `AIRTABLE_BASE_ID`: Required for Airtable base identification.
- `OPENAI_URL`: API endpoint (defaults to `https://api.openai.com/v1`).
- `MODEL_NAME`: AI model (defaults to `gpt-3.5-turbo-1106`).
- `PORT`: Server port (defaults to `8080`).

### API Structure:
```go
// Exercise Generation
POST /api/generate
{ "topic_id": "string" }
// -> Returns OpenAI completion response (JSON)

// Topics Management
GET    /api/topics      // Get all topics
POST   /api/topics      // Create a new topic
GET    /api/topics/{id} // Get a specific topic
PUT    /api/topics/{id} // Update a topic (creates a new version)
DELETE /api/topics/{id} // Delete a topic and its versions

// Version History
GET  /api/versions/{topicId}                  // Get version history for a topic
POST /api/versions/{topicId}/restore/{versionId} // Restore a specific version

// Observability
GET /api/last-refined-prompt // Get the most recently used refined prompt
```

## Airtable Integration

### Database Schema:
**Topics Table:**
- ID (Airtable record ID) 
- Name (Single line text)
- Prompt (Long text)
- CreatedAt (Single line text - RFC3339)
- UpdatedAt (Single line text - RFC3339)

**PromptVersions Table:**
- ID (Airtable record ID)
- TopicID (Single line text - foreign key)
- Prompt (Long text)
- Version (Number - sequential)
- CreatedAt (Single line text - RFC3339)

### Key Features:
- **Persistent Storage**: All topics and versions stored in Airtable
- **Version Management**: Automatic versioning (last 10 versions kept)
- **Permission Handling**: Graceful fallback if PromptVersions access unavailable
- **Default Topics**: Auto-creation on first startup (Conjunctions, Verb+Preposition, Preterite vs Perfect)
- **Duplicate Prevention**: Checks existing topics before creating defaults

## Frontend (app.js)
### Application State:
```javascript
state = {
  currentTopicId: '',         // Selected topic for exercise generation
  topics: [],                 // Array of available topics
  exercises: [],              // Array of exercise objects
  currentExerciseIndex: 0,    // Current position
  userSentence: [],           // User's constructed sentence
  isLocked: false,            // Prevent clicks during completion
  mistakes: 0,                // Session mistake count
  hintsUsed: 0,              // Session hint count
  startTime: null,            // Session start timestamp
  sessionTime: 0,            // Total session duration
  isSessionComplete: false,   // Session completion flag
  editingTopicId: null        // Currently editing topic ID
}
```

### Topics Management UI:
- **Topic Selector**: Dropdown to choose active topic for exercise generation
- **Topics List**: View, edit, delete existing topics
- **Topic Editor**: Modal for editing topic prompts with version history access
- **Add Topic Form**: Create new topics with name and custom prompt
- **Version History Modal**: View and restore previous prompt versions

### Exercise Object Structure:
```javascript
{
  conjunction_topic: "weil",
  english_hint: "He is learning German because...",
  correct_german_sentence: "Er lernt Deutsch, weil...",
  scrambled_words: ["er", "lernt", "Deutsch,", "weil", ...]
}
```

### Key Functions:
- `renderExercise()`: Displays current exercise with scrambled words
- `handleWordClick()`: Processes word selection and validation
- `handleSentenceCompletion()`: Manages exercise completion flow
- `showStatisticsPage()`: Displays final statistics after session completion
- `fetchExercises()`: Calls backend API to generate new exercises using selected topic
- `loadTopics()`: Fetches all topics from Airtable backend
- `createTopic()`: Creates new topic via API
- `updateTopicPrompt()`: Updates topic prompt (creates new version)
- `showVersionHistory()`: Displays version history modal for topic
- `restoreVersion()`: Restores a previous prompt version.
- `handleHintClick()`: Provides a hint to the user by highlighting the next correct word.
- `showStatisticsPage()`: Displays a detailed statistics page upon session completion.

### Key Features:
- **Local Word Scrambling**: The `renderExercise` function tokenizes the correct sentence and shuffles the words locally using a Fisher-Yates shuffle algorithm. This provides instant feedback without waiting for an API call.
- **Hint System**: The `handleHintClick` function highlights the next correct word in the sequence. Hint usage is tracked in the session statistics.
- **Statistics Tracking**: The application tracks mistakes, hints used, and session time. A detailed statistics page is shown at the end of a session.
- **Observability**: A "View Last Refined Prompt" button allows the user to see the prompt that was actually sent to the AI, which is useful for debugging.

## Prompt Refinement
The core of the application's exercise generation is the `refinePrompt` function in `main.go`. This function is designed to improve the quality and variety of exercises by using a two-step AI process:

1.  **Meta-Prompt**: A hardcoded `metaPrompt` is used to wrap the user's custom prompt. This meta-prompt instructs the language model to act as a "prompt engineering assistant" and refine the user's prompt based on a set of rules (e.g., enhance instructions, add examples, maintain the JSON schema).
2.  **Refined Prompt Generation**: The combined prompt (meta-prompt + user's prompt) is sent to the AI. The AI's response is the *refined prompt*.
3.  **Exercise Generation**: This new, refined prompt is then used to call the AI again to generate the actual exercises in the required JSON format.

This process happens automatically on every "Generate Exercises" request. If the refinement step fails, the system gracefully falls back to using the user's original prompt.

## Recent Changes
- **Prompt Refinement**: Added a system to automatically refine user prompts for better exercise quality.
- **Hint System**: Implemented a hint button and tracked hint usage.
- **Statistics Page**: Added a comprehensive session statistics page.
- **Local Word Scrambling**: Moved word scrambling from the AI to the frontend for instant feedback.
- **Rate Limiting**: Added server-side rate limiting to prevent abuse.
- **Observability**: Added a feature to view the last refined prompt.
- **Airtable Integration**: Added persistent storage for topics and prompt versions.

## Development Workflow
1. **Local Development**: `go run main.go` → http://localhost:8080
2. **Docker Build**: `docker-compose up`
3. **Cache Issues**: Server restart generates new timestamps
4. **API Testing**: Requires valid OpenAI API key in environment

## Frontend Dependencies
- **Tailwind CSS**: Via CDN for styling
- **Google Fonts**: Inter font family
- **No Build Process**: Pure vanilla JavaScript

## Known Considerations
- **Sample Data**: App initializes with sample exercises for testing
- **Error Handling**: API failures show alerts with error details
- **Keyboard Support**: Full hotkey navigation (1-9, a-z)
- **Responsive Design**: Mobile-friendly with Tailwind classes
- **State Persistence**: Only master prompt setting persists via localStorage

## Debugging Tips
- Console logging added for exercise completion flow
- Check browser developer tools for API response errors
- Cache issues resolved by server restart (generates new timestamps)
- Sample exercises automatically loaded for testing

## Future Enhancement Areas
- Exercise difficulty levels
- Performance analytics over time
- User authentication and progress saving
- More exercise types beyond sentence construction
- Offline mode support