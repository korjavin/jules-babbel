# German Conjunctions Trainer - Agent Documentation

## Project Overview
A web-based spaced repetition system (SRS) for learning German conjunctions. Users complete exercises by constructing sentences from scrambled words, with statistics tracking, performance feedback, and customizable topic management.

## Architecture
- **Backend**: Go HTTP server with OpenAI API integration and Airtable storage
- **Frontend**: Vanilla JavaScript with Tailwind CSS and topics management UI
- **Storage**: Airtable database for persistent topics and prompt versioning
- **Deployment**: Docker containerized with environment-based configuration

## File Structure
```
/
├── main.go           # Go backend server
├── index.html        # Main HTML page with Tailwind CSS styling
├── app.js           # Frontend JavaScript application
├── Dockerfile       # Container configuration
├── docker-compose.yml # Docker orchestration
├── go.mod/go.sum    # Go dependencies
└── agent.md         # This documentation file
```

## Backend (main.go)
### Key Components:
- **Static File Serving**: Custom handlers for dynamic cache-busting
- **API Endpoint**: `/api/generate` - Proxies requests to OpenAI API
- **Cache Management**: 
  - HTML files: no-cache headers
  - JS files: long-term caching with versioning
  - Dynamic timestamp injection for cache-busting

### Environment Variables:
- `OPENAI_API_KEY`: Required for AI exercise generation
- `AIRTABLE_TOKEN`: Required for Airtable integration (Personal Access Token)
- `AIRTABLE_BASE_ID`: Required for Airtable base identification  
- `OPENAI_URL`: API endpoint (defaults to OpenAI)
- `MODEL_NAME`: AI model (defaults to gpt-3.5-turbo-1106)
- `PORT`: Server port (defaults to 8080)

### API Structure:
```go
// Exercise Generation
POST /api/generate
{
  "topic_id": "string"
}
Response: OpenAI completion response (JSON)

// Topics Management
GET /api/topics - Get all topics
POST /api/topics - Create new topic
GET /api/topics/{id} - Get specific topic
PUT /api/topics/{id} - Update topic prompt (creates version)
DELETE /api/topics/{id} - Delete topic and versions

// Version History
GET /api/versions/{topicId} - Get version history
POST /api/versions/{topicId}/restore/{versionId} - Restore version
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
- `restoreVersion()`: Restores previous prompt version

### Statistics Features:
- **Tracked Metrics**: Exercises completed, mistakes, hints used, accuracy, time spent, avg time per exercise
- **Session Flow**: After completing all exercises, shows statistics page instead of cycling
- **Options**: Start new session or practice same exercises again

## Key Implementation Details

### Cache-Busting System:
1. Go server dynamically injects timestamps into HTML
2. HTML has no-cache headers
3. JS files cached with version parameters
4. Automatic cache invalidation on server restart

### Exercise Flow:
1. User loads page → Sample exercises or empty state
2. Click "Generate More Exercises" → API call to get new exercises
3. Complete exercises sequentially → Statistics page shown at end
4. Choose: new session or repeat same exercises

### Word Processing:
- **Tokenization**: Regex pattern `/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu`
- **Punctuation Handling**: Auto-added after correct words
- **Hotkeys**: Numbers 1-9, then letters a-z for quick selection
- **Visual Feedback**: Color-coded for correct/incorrect/hint states

### Settings System:
- **Master Prompt**: Stored in localStorage
- **Default Prompt**: Generates variable number of exercises (removed 7-exercise limit)
- **Modal Interface**: Settings accessible via header button

## Recent Changes
1. **Variable Exercise Support**: Removed hardcoded 7-exercise limit
2. **Statistics System**: Added comprehensive session tracking
3. **Session Completion**: Statistics page instead of infinite cycling
4. **Time Tracking**: Start-to-finish session timing
5. **Cache-Busting**: Dynamic versioning system for reliable updates

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