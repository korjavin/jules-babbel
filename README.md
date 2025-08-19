# German Conjunctions Trainer

An interactive German language learning application that helps B1-level students master German conjunctions through engaging word-scramble exercises.

## Features

- 🎯 Interactive word-scramble exercises
- ⌨️ Keyboard hotkeys (1-9, a-z) for quick word selection
- 🎨 Automatic punctuation handling
- 🔐 Secure backend API proxy (no client-side API keys)
- 🌍 Support for custom OpenAI-compatible APIs
- 📱 Responsive design

## Running with Docker

### Using the pre-built image from GHCR:

```bash
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=your_openai_api_key_here \
  -e OPENAI_URL=https://api.openai.com/v1 \
  -e MODEL_NAME=gpt-3.5-turbo-1106 \
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
  german-conjunctions-trainer
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | ✅ Yes | - | Your OpenAI API key or compatible API key |
| `OPENAI_URL` | ❌ No | `https://api.openai.com/v1` | API endpoint URL |
| `MODEL_NAME` | ❌ No | `gpt-3.5-turbo-1106` | Model name to use |
| `PORT` | ❌ No | `8080` | Port for the web server |

## Custom API Providers

The application supports any OpenAI-compatible API through environment variables:

```bash
# Example: Using Claude via Anthropic API
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=your_anthropic_key \
  -e OPENAI_URL=https://api.anthropic.com/v1 \
  -e MODEL_NAME=claude-3-sonnet-20240229 \
  german-conjunctions-trainer

# Example: Using Azure OpenAI
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=your_azure_key \
  -e OPENAI_URL=https://your-resource.openai.azure.com/v1 \
  -e MODEL_NAME=gpt-4 \
  german-conjunctions-trainer
```

## Development

### Local Development:

```bash
# Run the Go backend
go run main.go

# The server will serve static files from ./static/
# Access the app at http://localhost:8080
```

### Project Structure:

```
├── main.go              # Go backend server
├── index.html           # Main application UI
├── app.js              # Frontend JavaScript
├── Dockerfile          # Container definition
└── .github/workflows/  # CI/CD pipeline
```

## Security

- ✅ API keys are stored server-side only
- ✅ No sensitive data in browser localStorage
- ✅ CORS headers properly configured
- ✅ Non-root container user

## License

MIT License - see LICENSE file for details.