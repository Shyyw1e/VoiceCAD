package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Shyyw1e/VoiceCAD/internal/core"
	"github.com/Shyyw1e/VoiceCAD/internal/pipeline"
	"github.com/Shyyw1e/VoiceCAD/internal/storage"
)

type Server struct {
	users    core.UserRepository
	tasks    core.TaskRepository
	files    *storage.LocalStorage
	pipeline *pipeline.Pipeline
	log      *slog.Logger

	sessionsMu sync.RWMutex
	sessions   map[string]string
}

func NewServer(users core.UserRepository, tasks core.TaskRepository, files *storage.LocalStorage, pipe *pipeline.Pipeline, log *slog.Logger) *Server {
	return &Server{
		users:    users,
		tasks:    tasks,
		files:    files,
		pipeline: pipe,
		log:      log,
		sessions: make(map[string]string),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.Handle("POST /api/v1/tasks", s.auth(http.HandlerFunc(s.createTask)))
	mux.Handle("GET /api/v1/tasks", s.auth(http.HandlerFunc(s.listTasks)))
	mux.Handle("GET /api/v1/tasks/", s.auth(http.HandlerFunc(s.taskByID)))
	return s.recover(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, fmt.Errorf("valid email is required"))
		return
	}

	passwordHash, err := core.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	now := time.Now().UTC()
	user := core.User{
		ID:           core.NewID("usr"),
		Email:        email,
		Name:         strings.TrimSpace(req.Name),
		PasswordHash: passwordHash,
		CreatedAt:    now,
	}
	if user.Name == "" {
		user.Name = email
	}

	if err := s.users.Create(user); err != nil {
		if errors.Is(err, core.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, fmt.Errorf("user already exists"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	user, err := s.users.FindByEmail(strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil || !core.VerifyPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid email or password"))
		return
	}

	token := core.NewToken()
	s.sessionsMu.Lock()
	s.sessions[token] = user.ID
	s.sessionsMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	user := userFromRequest(r)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	platform := core.CADPlatform(r.FormValue("cad_platform"))
	if platform == "" {
		platform = core.CADPlatformKompas
	}
	if platform != core.CADPlatformKompas && platform != core.CADPlatformTFlex {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported cad_platform"))
		return
	}

	taskID := core.NewID("tsk")
	var audioPath string
	file, header, err := r.FormFile("audio")
	if err == nil {
		defer file.Close()
		audioPath, err = s.files.Save("audio", taskID+"_"+filepath.Base(header.Filename), file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else if r.FormValue("text") == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("audio file or text field is required"))
		return
	}

	now := time.Now().UTC()
	task := core.Task{
		ID:           taskID,
		UserID:       user.ID,
		Status:       core.TaskStatusQueued,
		CADPlatform:  platform,
		AudioPath:    audioPath,
		OriginalText: strings.TrimSpace(r.FormValue("text")),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.tasks.Create(task); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.pipeline.Enqueue(task.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"task": task})
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	user := userFromRequest(r)
	tasks, err := s.tasks.ListByUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) taskByID(w http.ResponseWriter, r *http.Request) {
	user := userFromRequest(r)
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	taskID, action, _ := strings.Cut(path, "/")
	if taskID == "" {
		writeError(w, http.StatusNotFound, core.ErrNotFound)
		return
	}

	task, err := s.tasks.FindByID(taskID)
	if err != nil || task.UserID != user.ID {
		writeError(w, http.StatusNotFound, core.ErrNotFound)
		return
	}

	if action == "download" {
		s.downloadResult(w, r, task)
		return
	}
	if action != "" {
		writeError(w, http.StatusNotFound, core.ErrNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

func (s *Server) downloadResult(w http.ResponseWriter, r *http.Request, task core.Task) {
	if task.Status != core.TaskStatusDone || task.ResultPath == "" {
		writeError(w, http.StatusConflict, fmt.Errorf("result is not ready"))
		return
	}

	file, err := s.files.Open(task.ResultPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(task.ResultPath)+`"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = io.Copy(w, file)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("bearer token is required"))
			return
		}

		s.sessionsMu.RLock()
		userID, ok := s.sessions[token]
		s.sessionsMu.RUnlock()
		if !ok {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
			return
		}

		user, err := s.users.FindByID(userID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
			return
		}

		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
	})
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic recovered", "value", recovered)
				writeError(w, http.StatusInternalServerError, fmt.Errorf("internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
