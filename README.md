# Impact

An AI-powered video processing API that detects high-impact moments in short-form video content and applies dramatic visual effects at those moments. It uses Google Gemini's vision capabilities to analyze video frame grids, identify frames where significant physical contact occurs, and applies a high-contrast B&W effect to those moments — all while preserving the original video's native framerate.

Built for anime fight scenes, sports highlights, movie clips, and similar content where styled impact frames add visual punch.

## Example Output

[Watch an example processed video on Google Drive](https://drive.google.com/file/d/1U9a3cpHXVYLg69MEpGD4vF-DuBEgP_j2/view?usp=sharing)

## How It Works

1. A video is uploaded via the REST API.
2. FFmpeg extracts frames at 10fps and burns a visible frame number label onto each one.
3. The labeled frames are arranged into 5x4 grids (20 frames per grid) to reduce the number of images sent to the model.
4. The grid images are uploaded to Google Gemini via the File API.
5. Gemini analyzes all grids and returns a JSON array of frame numbers where key action moments (physical contact, collisions, strikes) occur.
6. The frame numbers are converted to timestamps, and FFmpeg applies a high-contrast B&W effect directly to the original video at those moments (±2 frames), preserving the native framerate.
7. The processed video is served as a static download.

## Architecture

The project follows a hexagonal (ports and adapters) layout:

```
cmd/main.go                          Entry point
internals/
  configs/
    connections/                      Gemini client init
    env/                              Environment variable loading
    errors/                           Custom error types (app + Postgres)
    file/                             Root path resolution
    goth/                             OAuth provider config (Google, GitHub)
    prompts/system.md                 System prompt for Gemini
    response/                         Standardized JSON response wrappers
  infrastructures/
    adapters/
      adapters.go                     Wires DB queries + LLM adapter
      llm/command.go                  Gemini API adapter (upload, generate, cost calc)
      user/command.go                 User adapter (placeholder)
      video/command.go                FFmpeg operations (extract, label, grid, effects)
    db/
      db.go                           pgxpool creation
      gen/                            sqlc-generated query code
      migrations/001_users.sql        Goose migration for users table
      queries/users.sql               SQL for user CRUD
    domain/
      llm/                            LLM types (Response, File, UploadedFile) + interface
      video/                          Video types (ImpactResponse with frame numbers)
    ports/
      ports.go                        Port wrapper
      http/
        gin.go                        Gin server, CORS, routing
        handlers/ffmpeg.go            Core pipeline handler
        middlewears/authentication.go  Auth middleware (placeholder)
  services/
    services.go                       Service layer (DB queries + LLM interface)
    ffmpeg/ffmpeg.go                  FFmpeg service (placeholder)
pkg/
  utils/retry.go                      Generic retry with exponential backoff
```

## Prerequisites

- **Go 1.26+**
- **FFmpeg and FFprobe** installed and available on PATH
- **PostgreSQL** database
- **Google Gemini API key** with access to a vision-capable model

## Environment Variables

Create a `.env` file in the project root. All variables marked as required will cause a panic on startup if missing.

### Required

| Variable               | Description                                       |
| ---------------------- | ------------------------------------------------- |
| `GOOGLE_API_KEY`       | Google Gemini API key                             |
| `GEMINI_MODEL`         | Primary Gemini model name (e.g. `gemini-2.5-pro`) |
| `GEMINI_FAST_MODEL`    | Fast/cheap model for lighter tasks                |
| `GEMINI_LIVE_MODEL`    | Live model name                                   |
| `JWT_SECRET`           | Secret for signing JWTs                           |
| `REFRESH_JWT_SECRET`   | Secret for refresh tokens                         |
| `COOKIE_SECRET`        | Cookie signing secret                             |
| `SESSIONS_SECRET`      | Session store secret                              |
| `POSTGRES_PASSWORD`    | PostgreSQL password                               |
| `POSTGRES_DB`          | PostgreSQL database name                          |
| `DB_URL`               | Full Postgres connection string for pgxpool       |
| `GOOGLE_CLIENT_ID`     | Google OAuth client ID                            |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret                        |
| `GOOGLE_CALLBACK_URL`  | Google OAuth callback URL                         |
| `GITHUB_CLIENT_ID`     | GitHub OAuth client ID                            |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth client secret                        |
| `GITHUB_CALLBACK_URL`  | GitHub OAuth callback URL                         |
| `SMTP_FROM_ADDRESS`    | Sender email address                              |
| `SMTP_HOST`            | SMTP server host                                  |
| `SMTP_USERNAME`        | SMTP username                                     |
| `SMTP_PASSWORD`        | SMTP password                                     |

### Optional

| Variable                 | Default     | Description            |
| ------------------------ | ----------- | ---------------------- |
| `PORT`                   | `:5000`     | Server listen address  |
| `POSTGRES_USER`          | `postgres`  | PostgreSQL username    |
| `POSTGRES_HOST`          | `127.0.0.1` | PostgreSQL host        |
| `POSTGRES_PORT`          | `5432`      | PostgreSQL port        |
| `POSTGRES_SSL`           | `false`     | PostgreSQL SSL mode    |
| `SMTP_PORT`              | `587`       | SMTP port              |
| `PRODUCTION_ENVIRONMENT` | `false`     | Toggle production mode |

## Setup

```bash
# Clone the repository
git clone https://github.com/umohsamuel/impact.git
cd impact

# Install Go dependencies
go mod download

# Set up the database (using goose or manual migration)
# The migration file is at internals/infrastructures/db/migrations/001_users.sql

# Create your .env file
cp .env.example .env  # then fill in your values

# Build and run
go build -o ./tmp/main.exe ./cmd
./tmp/main.exe
```

For development with hot reload using [Air](https://github.com/air-verse/air):

```bash
air
```

The Air config (`.air.toml`) builds to `tmp/main.exe` and watches for changes.

## API Endpoints

### Health Check

```
GET /health
```

Returns a simple status message confirming the server is running.

### Generate Impact Video

```
POST /api/v1/generate-impact-frames
Content-Type: multipart/form-data
```

**Form fields:**

| Field   | Type | Required | Description           |
| ------- | ---- | -------- | --------------------- |
| `video` | file | Yes      | Video file (max 1 GB) |

Frames are extracted at a fixed rate of 10fps. No sample rate parameter is needed.

**Response:**

```json
{
  "status": "success",
  "message": "Impact video generated successfully",
  "data": {
    "download_url": "/downloads/{sessionID}/output.mp4",
    "impacts": [12, 45, 78],
    "llm_cost": 0.003421
  }
}
```

### Download Processed Video

```
GET /downloads/{sessionID}/{filename}
```

Static file serving from the `tmp/` directory. The download URL is included in the generate response.

## Impact Detection

The system prompt instructs Gemini to analyze 5x4 frame grids and identify the exact frames where physical contact occurs. The detection process:

1. **Scene understanding** -- look at all grids to understand the full video context.
2. **Contact identification** -- find frames where two things make significant physical contact (strikes connecting, collisions, objects breaking, projectiles hitting targets).
3. **Rejection filters** -- each candidate is checked:
   - No actual contact between objects? Reject.
   - Black screen, white screen, or solid color? Reject.
   - Scene transition, cutaway, or text overlay? Reject.
   - Wind-up or aftermath rather than the contact frame? Reject.
   - Movement without contact (running, jumping)? Reject.
4. **Final selection** -- only the 2-5 best, clearest impact frames are returned.

The model returns a simple JSON object: `{"impacts": [12, 45, 78]}` where each number corresponds to a labeled frame.

## Visual Effects

Impact frames (±2 frames around each detected moment) receive a dramatic B&W effect applied directly to the original video stream:

- **Full desaturation** via `hue=s=0`
- **High contrast + brightness boost** via `eq=contrast=2.0:brightness=0.1`
- **Custom tone curve** via `curves` for a punchy, manga-style look

Effects are applied using FFmpeg's `enable='between(t,...)'` expressions, which means the original video's native framerate is preserved — no frame extraction or reassembly needed for the output.

## LLM Cost Tracking

The adapter tracks token usage and calculates cost per request using a built-in pricing table covering Gemini 2.0, 2.5, 3.0, and 3.1 model variants. The cost in dollars is returned in every response.

## Dependencies

Core dependencies from `go.mod`:

- [gin-gonic/gin](https://github.com/gin-gonic/gin) -- HTTP framework
- [gin-contrib/cors](https://github.com/gin-contrib/cors) -- CORS middleware
- [jackc/pgx/v5](https://github.com/jackc/pgx) -- PostgreSQL driver and connection pool
- [joho/godotenv](https://github.com/joho/godotenv) -- .env file loading
- [markbates/goth](https://github.com/markbates/goth) -- OAuth providers (Google, GitHub)
- [google.golang.org/genai](https://pkg.go.dev/google.golang.org/genai) -- Google Gemini API client

## License

See [LICENSE](LICENSE) for details.
