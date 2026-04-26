# Impact

An AI-powered video processing API that detects high-impact moments in short-form video content and applies cinematic freeze-frame effects at those moments. It uses Google Gemini's vision capabilities to analyze extracted video frames, identify points of forceful contact (punches, collisions, strikes, energy blasts, etc.), and then uses FFmpeg to splice stylized freeze-frame clips into the video at each impact timestamp.

Built for anime fight scenes, sports highlights, movie clips, and similar content where brief paused-and-styled frames at the moment of contact can add visual punch.

## How It Works

1. A video is uploaded via the REST API.
2. FFmpeg extracts frames at a configurable sample rate (default: 1 frame/sec).
3. The extracted frames are uploaded to Google Gemini.
4. Gemini analyzes the full sequence of frames using a detailed system prompt that enforces a force-transfer test and multiple rejection filters (black screens, transitions, cutaways, etc.).
5. The model returns structured JSON with timestamps, confidence scores, and per-impact effect parameters (freeze duration, invert strength, B&W contrast, zoom, vignette).
6. The pipeline splits the original video into segments around each impact timestamp, generates a short freeze-frame clip with the specified visual effects applied via FFmpeg filters, and concatenates everything back together.
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
      video/command.go                FFmpeg operations (extract, metadata, effects, concat)
    db/
      db.go                           pgxpool creation
      gen/                            sqlc-generated query code
      migrations/001_users.sql        Goose migration for users table
      queries/users.sql               SQL for user CRUD
    domain/
      llm/                            LLM types (Response, File, UploadedFile) + interface
      video/                          Video types (ImpactFrame, effect configs, analysis response)
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

### Generate Impact Frames

```
POST /api/v1/generate-impact-frames
Content-Type: multipart/form-data
```

**Form fields:**

| Field         | Type  | Required | Description                                                |
| ------------- | ----- | -------- | ---------------------------------------------------------- |
| `video`       | file  | Yes      | Video file (max 1 GB)                                      |
| `sample_rate` | float | No       | Frames per second to extract for analysis (default: `1.0`) |

**Response:**

```json
{
  "status": "success",
  "message": "Impact video generated successfully",
  "data": {
    "download_url": "/downloads/{sessionID}/output.mp4",
    "analysis": {
      "video_analysis": {
        "total_impacts_detected": 3,
        "overall_intensity": "high",
        "content_type": "action",
        "processing_notes": "Fight scene with multiple strikes"
      },
      "impact_frames": [
        {
          "impact_id": 1,
          "frame_index": 45,
          "timestamp_ms": 1500,
          "impact_type": "physical",
          "impact_label": "Right hook connects",
          "confidence": 0.95,
          "intensity": "strong",
          "freeze_frame": { "enabled": true, "freeze_duration_ms": 150 },
          "invert_filter": { "enabled": true, "invert_strength": 0.6 },
          "bw_filter": {
            "enabled": false,
            "bw_intensity": 0,
            "contrast_boost": 1
          },
          "zoom": {
            "enabled": true,
            "zoom_factor": 1.15,
            "zoom_duration_ms": 100
          },
          "vignette": { "enabled": true, "vignette_strength": 0.5 },
          "slowdown": { "enabled": false }
        }
      ]
    },
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

The system prompt instructs Gemini to follow a strict detection process:

1. **Scene understanding** -- look at the full frame sequence to understand context.
2. **Candidate identification** -- find moments where two things make contact with force.
3. **Rejection filters** -- each candidate is checked against multiple filters:
   - No actual contact between objects? Reject.
   - Black screen, white screen, or solid color? Reject.
   - Scene transition, cutaway, or text overlay? Reject.
   - Wind-up or aftermath rather than the contact frame? Reject.
   - Too weak or ambiguous? Reject.
4. **Final selection** -- only the strongest, cleanest impacts survive. Quality over quantity.

The core test applied to every candidate: "Are two things making forceful contact right now?" If not, it gets rejected.

## Visual Effects

Each detected impact gets a short freeze-frame clip with configurable FFmpeg filters:

- **Freeze frame**: A brief pause (80-350ms) at the moment of contact.
- **Invert/high-contrast**: A washed-out, high-contrast look. Partial desaturation, moderate contrast boost, gentle curve adjustment. Subjects remain visible and recognizable.
- **Black and white**: Desaturation with configurable contrast boost for a stark, punchy look.
- **Zoom**: A quick scale punch that zooms in slightly on the impact point.
- **Vignette**: Darkened edges for cinematic framing.

The model decides which effects to enable and at what intensity for each impact based on how powerful the hit looks.

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

## Database

PostgreSQL with [sqlc](https://sqlc.dev/) for type-safe query generation. The current schema has a single `users` table:

```sql
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  name TEXT NOT NULL
);
```

An `updated_at` trigger automatically updates the timestamp on row changes. Migrations are managed with [Goose](https://github.com/pressly/goose).

## License

See [LICENSE](LICENSE) for details.
