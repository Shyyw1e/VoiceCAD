package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Shyyw1e/VoiceCAD/internal/core"
	"github.com/jackc/pgx/v5/pgconn"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user core.User) error {
	_, err := r.db.ExecContext(
		context.Background(),
		`INSERT INTO users (id, email, name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		user.ID,
		user.Email,
		user.Name,
		user.PasswordHash,
		user.CreatedAt,
	)
	if isUniqueViolation(err) {
		return core.ErrAlreadyExists
	}
	return err
}

func (r *UserRepository) FindByEmail(email string) (core.User, error) {
	return r.findOne(`SELECT id, email, name, password_hash, created_at FROM users WHERE email = $1`, email)
}

func (r *UserRepository) FindByID(id string) (core.User, error) {
	return r.findOne(`SELECT id, email, name, password_hash, created_at FROM users WHERE id = $1`, id)
}

func (r *UserRepository) findOne(query string, args ...any) (core.User, error) {
	var user core.User
	err := r.db.QueryRowContext(context.Background(), query, args...).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.User{}, core.ErrNotFound
	}
	return user, err
}

type TaskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(task core.Task) error {
	commandJSON, err := marshalCommand(task.ParsedCommand)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(
		context.Background(),
		`INSERT INTO tasks (
			id, user_id, status, cad_platform, audio_path, result_path, original_text,
			parsed_command, error, created_at, updated_at, completed_at
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8::jsonb, NULLIF($9, ''), $10, $11, $12)`,
		task.ID,
		task.UserID,
		task.Status,
		task.CADPlatform,
		task.AudioPath,
		task.ResultPath,
		task.OriginalText,
		commandJSON,
		task.Error,
		task.CreatedAt,
		task.UpdatedAt,
		task.CompletedAt,
	)
	if isUniqueViolation(err) {
		return core.ErrAlreadyExists
	}
	return err
}

func (r *TaskRepository) Update(task core.Task) error {
	commandJSON, err := marshalCommand(task.ParsedCommand)
	if err != nil {
		return err
	}

	result, err := r.db.ExecContext(
		context.Background(),
		`UPDATE tasks SET
			status = $2,
			cad_platform = $3,
			audio_path = NULLIF($4, ''),
			result_path = NULLIF($5, ''),
			original_text = NULLIF($6, ''),
			parsed_command = $7::jsonb,
			error = NULLIF($8, ''),
			updated_at = $9,
			completed_at = $10
		WHERE id = $1`,
		task.ID,
		task.Status,
		task.CADPlatform,
		task.AudioPath,
		task.ResultPath,
		task.OriginalText,
		commandJSON,
		task.Error,
		task.UpdatedAt,
		task.CompletedAt,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *TaskRepository) FindByID(id string) (core.Task, error) {
	return r.findOne(`SELECT `+taskColumns+` FROM tasks WHERE id = $1`, id)
}

func (r *TaskRepository) ListByUser(userID string) ([]core.Task, error) {
	rows, err := r.db.QueryContext(
		context.Background(),
		`SELECT `+taskColumns+` FROM tasks WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]core.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *TaskRepository) findOne(query string, args ...any) (core.Task, error) {
	row := r.db.QueryRowContext(context.Background(), query, args...)
	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Task{}, core.ErrNotFound
	}
	return task, err
}

const taskColumns = `
	id, user_id, status, cad_platform, audio_path, result_path, original_text,
	parsed_command, error, created_at, updated_at, completed_at
`

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (core.Task, error) {
	var task core.Task
	var audioPath sql.NullString
	var resultPath sql.NullString
	var originalText sql.NullString
	var parsedCommand []byte
	var taskError sql.NullString
	var completedAt sql.NullTime

	err := row.Scan(
		&task.ID,
		&task.UserID,
		&task.Status,
		&task.CADPlatform,
		&audioPath,
		&resultPath,
		&originalText,
		&parsedCommand,
		&taskError,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
	)
	if err != nil {
		return core.Task{}, err
	}

	task.AudioPath = audioPath.String
	task.ResultPath = resultPath.String
	task.OriginalText = originalText.String
	task.Error = taskError.String
	task.ResultAvailable = task.ResultPath != ""
	if completedAt.Valid {
		completed := completedAt.Time
		task.CompletedAt = &completed
	}
	if len(parsedCommand) > 0 {
		if err := json.Unmarshal(parsedCommand, &task.ParsedCommand); err != nil {
			return core.Task{}, err
		}
	}

	return task, nil
}

func marshalCommand(command core.Command) (any, error) {
	if command.Intent == "" && command.Primitive == "" && command.Units == "" && len(command.Parameters) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}

	return strings.Contains(err.Error(), "SQLSTATE 23505")
}

var _ core.UserRepository = (*UserRepository)(nil)
var _ core.TaskRepository = (*TaskRepository)(nil)
