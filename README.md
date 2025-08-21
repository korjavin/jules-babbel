# German Conjunctions Trainer

An interactive German language learning application that helps B1-level students master German conjunctions through engaging word-scramble exercises.

## Features

- 🎯 Interactive word-scramble exercises with customizable topics
- ⌨️ Keyboard hotkeys (1-9, a-z) for quick word selection
- 🎨 Automatic punctuation handling
- 🔐 Secure backend API proxy (no client-side API keys)
- 🌍 Support for custom OpenAI-compatible APIs
- 📱 Responsive design
- 🏷️ **Topics Management**: Create and organize different grammar topics
- 📝 **Prompt Customization**: Edit and customize exercise generation prompts
- 🕒 **Version History**: Track and restore previous prompt versions (last 10)
- 💾 **Airtable Integration**: Persistent storage for topics and prompts

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

### Project Structure:

```
├── main.go              # Go backend server with Airtable integration
├── index.html           # Main application UI with topics management
├── app.js              # Frontend JavaScript with topics & versioning
├── agent.md            # Context file for AI development
├── Dockerfile          # Container definition
├── docker-compose.yml  # Docker composition with environment variables
└── .github/workflows/  # CI/CD pipeline
```

### Topics Management Features:

- **Create Topics**: Add new grammar topics with custom prompts
- **Edit Prompts**: Modify exercise generation prompts for each topic  
- **Version History**: Automatic versioning of prompt changes (last 10 versions)
- **Restore Versions**: Easily revert to previous prompt versions
- **Delete Topics**: Remove topics and all associated versions
- **Persistent Storage**: All data stored in Airtable for persistence

## Security

- ✅ API keys are stored server-side only
- ✅ No sensitive data in browser localStorage
- ✅ CORS headers properly configured
- ✅ Non-root container user

## License

MIT License - see LICENSE file for details.