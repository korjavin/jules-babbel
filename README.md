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
  ghcr.io/YOUR_USERNAME/german-conjuctions-trainer:latest
```

### Building locally:

```bash
# Build the image
docker build -t german-conjunctions-trainer .

# Run the container
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=your_openai_api_key_here \
  german-conjunctions-trainer
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | ✅ Yes | - | Your OpenAI API key or compatible API key |
| `PORT` | ❌ No | `8080` | Port for the web server |

## Custom API Providers

The application supports any OpenAI-compatible API. Configure through the settings UI:

- **OpenAI API URL**: Custom API endpoint (e.g., `https://api.anthropic.com/v1` for Claude)
- **Model Name**: Specific model to use (e.g., `gpt-4`, `claude-3-sonnet`, etc.)

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