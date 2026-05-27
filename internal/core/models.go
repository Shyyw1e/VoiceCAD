package core

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type TaskStatus string

const (
	TaskStatusQueued       TaskStatus = "queued"
	TaskStatusTranscribing TaskStatus = "transcribing"
	TaskStatusParsing      TaskStatus = "parsing"
	TaskStatusExecutingCAD TaskStatus = "executing_cad"
	TaskStatusDone         TaskStatus = "done"
	TaskStatusFailed       TaskStatus = "failed"
)

type CADPlatform string

const (
	CADPlatformKompas CADPlatform = "kompas3d"
	CADPlatformTFlex  CADPlatform = "tflex"
)

type Task struct {
	ID              string      `json:"id"`
	UserID          string      `json:"user_id"`
	Status          TaskStatus  `json:"status"`
	CADPlatform     CADPlatform `json:"cad_platform"`
	AudioPath       string      `json:"-"`
	ResultPath      string      `json:"-"`
	OriginalText    string      `json:"original_text,omitempty"`
	ParsedCommand   Command     `json:"parsed_command,omitempty"`
	Error           string      `json:"error,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	CompletedAt     *time.Time  `json:"completed_at,omitempty"`
	ResultAvailable bool        `json:"result_available"`
}

type Command struct {
	Intent     string         `json:"intent"`
	Primitive  string         `json:"primitive"`
	Parameters map[string]any `json:"parameters"`
	Units      string         `json:"units"`
}
