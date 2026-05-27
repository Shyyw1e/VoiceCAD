# VoiceCAD Backend

Go backend MVP for VoiceCAD: user auth, task creation, audio/text input, async processing pipeline, file storage, Telegram UX, and integration points for ML and CAD worker services.

## Run Locally

```powershell
$env:GOCACHE='C:\Users\Shyywie\go_projects\VoiceCAD\.gocache'
go run ./cmd/voicecad
```

The service reads `.env` automatically when it exists.

## Docker Compose

```powershell
docker compose up --build
```

Compose starts:

- `postgres` on host port `5432`;
- `app` on host port `8080`.

The app uses `.env`, but inside Docker Compose `POSTGRES_HOST` is overridden to `postgres`.
Storage and database files are kept in named Docker volumes: `voicecad_app_storage` and `voicecad_postgres_data`.

Useful environment variables:

```env
HTTP_ADDR=:8080
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_DB=voicecad
POSTGRES_SSLMODE=disable
STORAGE_LOCAL_DIR=data/storage
ML_TRANSCRIBER_URL=
ML_PARSER_URL=
CAD_EXECUTOR_URL=
TELEGRAM_BOT_TOKEN=
```

PostgreSQL is required on startup. Migrations are embedded into the binary and applied automatically from `internal/postgres/migrations`.

When ML/CAD URLs are empty, the backend runs in demo mode and creates a text result file locally.
When `TELEGRAM_BOT_TOKEN` is set, the service also starts a Telegram bot through long polling.

## Telegram UX

The bot supports `/start`, `/help`, text CAD commands, voice messages, audio files, and document uploads for audio-like input.
For every incoming command, the bot creates a backend task, waits for completion, and sends the result file back to the same chat.

## API

### Health

```http
GET /health
```

### Register

```http
POST /api/v1/auth/register
Content-Type: application/json

{
  "email": "student@example.com",
  "name": "Student",
  "password": "password123"
}
```

### Login

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "student@example.com",
  "password": "password123"
}
```

Response contains `token`. Use it as `Authorization: Bearer <token>`.

### Create Task

```http
POST /api/v1/tasks
Authorization: Bearer <token>
Content-Type: multipart/form-data

audio=<file>
cad_platform=kompas3d
```

For demo/testing without audio, send `text` instead:

```http
POST /api/v1/tasks
Authorization: Bearer <token>
Content-Type: multipart/form-data

text=create rectangular plate 100 by 50 with thickness 5 mm
cad_platform=kompas3d
```

Supported `cad_platform` values: `kompas3d`, `tflex`.

### List Tasks

```http
GET /api/v1/tasks
Authorization: Bearer <token>
```

### Get Task

```http
GET /api/v1/tasks/{task_id}
Authorization: Bearer <token>
```

### Download Result

```http
GET /api/v1/tasks/{task_id}/download
Authorization: Bearer <token>
```

## ML/CAD Contracts

### Transcriber

Configured by `ML_TRANSCRIBER_URL`.

Request:

```json
{
  "audio_path": "data/storage/audio/tsk_..._voice.wav"
}
```

Response:

```json
{
  "text": "create rectangular plate 100 by 50 with thickness 5 mm"
}
```

### Parser / LLM

Configured by `ML_PARSER_URL`.

Request:

```json
{
  "text": "create rectangular plate 100 by 50 with thickness 5 mm"
}
```

Response:

```json
{
  "intent": "create_part",
  "primitive": "plate",
  "units": "mm",
  "parameters": {
    "length": 100,
    "width": 50,
    "thickness": 5
  }
}
```

### CAD Executor

Configured by `CAD_EXECUTOR_URL`.

Request:

```json
{
  "task_id": "tsk_...",
  "cad_platform": "kompas3d",
  "command": {
    "intent": "create_part",
    "primitive": "plate",
    "units": "mm",
    "parameters": {
      "length": 100,
      "width": 50,
      "thickness": 5
    }
  }
}
```

Response:

```json
{
  "result_path": "data/storage/results/tsk_....m3d"
}
```
