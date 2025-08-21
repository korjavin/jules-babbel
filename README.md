# German Conjunctions Trainer

An interactive German language learning application that helps B1-level students master German grammar through engaging word-scramble exercises.

## Features

- ✨ **Automatic Prompt Refinement**: Uses a meta-prompt to automatically improve user-defined prompts, leading to more creative and varied exercises.
- 🎯 **Interactive Exercises**: Engaging word-scramble exercises with customizable topics.
- 💡 **Hint System**: Provides hints for the next correct word, with usage tracking.
- 📈 **Session Statistics**: Detailed performance tracking, including mistakes, hints used, accuracy, and time per exercise.
-  Lokal **Word Scrambling**: Ensures instant feedback by scrambling words locally.
- ⌨️ **Keyboard Hotkeys**: Use keys 1-9 and a-z for quick word selection.
- 🎨 **Automatic Punctuation**: Handles punctuation automatically for a smoother experience.
- 🔐 **Secure Backend**: API keys are stored securely on the server-side.
- 🌍 **Custom API Support**: Compatible with any OpenAI-compatible API.
- 📱 **Responsive Design**: Fully functional on both desktop and mobile devices.
- 🏷️ **Topics Management**: Create, edit, and delete grammar topics.
- 📝 **Prompt Customization**: Tailor exercise generation prompts for each topic.
- 🕒 **Version History**: Track and restore the last 10 versions of a prompt.
- 💾 **Airtable Integration**: Persistently stores topics and prompt versions.

## Prompt Refinement

This application uses a unique **Prompt Refinement** feature to enhance the quality of the generated exercises. When you request new exercises, the application first sends your custom prompt to a language model with a "meta-prompt". This meta-prompt instructs the model to refine your original prompt for better clarity, creativity, and variety, all while preserving the core task and required JSON output format.

This ensures that the exercises you receive are not repetitive and are of higher pedagogical quality.

## Observability

To provide insight into the prompt refinement process, you can view the most recently used refined prompt. This is useful for debugging and understanding how the AI is interpreting and improving your prompts.

You can access this feature via the "View Last Refined Prompt" button in the settings menu.

## Running with Docker

### Using the pre-built image from GHCR:

```bash
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=your_openai_api_key_here \
  -e OPENAI_URL=https://api.openai.com/v1 \
  -e MODEL_NAME=gpt-3.5-turbo-1106 \
  -e AIRTABLE_TOKEN=your_airtable_token \
  -e AIRTABLE_BASE_ID=your_base_id \
  ghcr.io/YOUR_USERNAME/german-conjuctions-trainer:latest
```

### Building locally:

```bash
# Build the image
docker build -t german-conjunctions-trainer .

# Run the container
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=your_openai_api_key_here \
  -e OPENAI_URL=https://api.openai.com/v1 \
  -e MODEL_NAME=gpt-4 \
  -e AIRTABLE_TOKEN=your_airtable_token \
  -e AIRTABLE_BASE_ID=your_base_id \
  german-conjunctions-trainer
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | ✅ Yes | - | Your OpenAI API key or compatible API key |
| `AIRTABLE_TOKEN` | ✅ Yes | - | Your Airtable Personal Access Token |
| `AIRTABLE_BASE_ID` | ✅ Yes | - | Your Airtable Base ID |
| `OPENAI_URL` | ❌ No | `https://api.openai.com/v1` | API endpoint URL |
| `MODEL_NAME` | ❌ No | `gpt-3.5-turbo-1106` | Model name to use |
| `PORT` | ❌ No | `8080` | Port for the web server |

## Airtable Setup

The application uses Airtable for persistent storage of topics and prompt versions. You need to:

### 1. Create an Airtable Base

Create a new base in Airtable and note the Base ID from the URL.

### 2. Create Required Tables

Create these two tables in your Airtable base:

**Table 1: "Topics"**
- `Name` - Single line text (required)
- `Prompt` - Long text (required) 
- `CreatedAt` - Single line text (optional)
- `UpdatedAt` - Single line text (optional)

**Table 2: "PromptVersions"**
- `TopicID` - Single line text (required)
- `Prompt` - Long text (required)
- `Version` - Number (required)
- `CreatedAt` - Single line text (optional)

### 3. Generate Personal Access Token

1. Go to [Airtable Developer Hub](https://airtable.com/create/tokens)
2. Create a new Personal Access Token
3. Grant the following permissions:
   - `data.records:read` (for both tables)
   - `data.records:write` (for both tables)
4. Select your specific base

### 4. Environment Variables

Set the required environment variables:
```bash
export AIRTABLE_TOKEN="patXXXXXXXXXXXXXX"
export AIRTABLE_BASE_ID="appXXXXXXXXXXXXXX"
```

### Default Topics

On first startup, the application will create three default topics:
- **Conjunctions**: Focus on German conjunctions (weil, obwohl, etc.)
- **Verb + Preposition**: Verb-preposition combinations
- **Preterite vs Perfect**: Practice with German tenses

## Custom API Providers

The application supports any OpenAI-compatible API through environment variables:

```bash
# Example: Using Claude via Anthropic API
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=your_anthropic_key \
  -e OPENAI_URL=https://api.anthropic.com/v1 \
  -e MODEL_NAME=claude-3-sonnet-20240229 \
  -e AIRTABLE_TOKEN=your_airtable_token \
  -e AIRTABLE_BASE_ID=your_base_id \
  german-conjunctions-trainer

# Example: Using Azure OpenAI
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=your_azure_key \
  -e OPENAI_URL=https://your-resource.openai.azure.com/v1 \
  -e MODEL_NAME=gpt-4 \
  -e AIRTABLE_TOKEN=your_airtable_token \
  -e AIRTABLE_BASE_ID=your_base_id \
  german-conjunctions-trainer
```

## Development

### Local Development:

```bash
# Set required environment variables
export OPENAI_API_KEY=your_openai_api_key
export AIRTABLE_TOKEN=your_airtable_token  
export AIRTABLE_BASE_ID=your_base_id

# Run the Go backend
go run main.go

# The server will serve static files from ./static/ (for Docker) or current directory (local)
# Access the app at http://localhost:8080
```

### Rate Limiting
The backend includes rate limiting to prevent abuse. By default, it allows one request every three seconds per IP address.

### Project Structure

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

## Security

- ✅ API keys are stored server-side only
- ✅ No sensitive data in browser localStorage
- ✅ CORS headers properly configured
- ✅ Non-root container user

## License

MIT License - see LICENSE file for details.