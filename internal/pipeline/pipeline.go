package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Shyyw1e/VoiceCAD/internal/core"
	"github.com/Shyyw1e/VoiceCAD/internal/storage"
)

type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string) (string, error)
}

type Parser interface {
	Parse(ctx context.Context, text string) (core.Command, error)
}

type CADExecutor interface {
	Execute(ctx context.Context, task core.Task) (string, error)
}

type Pipeline struct {
	tasks       core.TaskRepository
	storage     *storage.LocalStorage
	transcriber Transcriber
	parser      Parser
	executor    CADExecutor
	queue       chan string
	log         *slog.Logger
}

func New(tasks core.TaskRepository, files *storage.LocalStorage, transcriber Transcriber, parser Parser, executor CADExecutor, log *slog.Logger) *Pipeline {
	return &Pipeline{
		tasks:       tasks,
		storage:     files,
		transcriber: transcriber,
		parser:      parser,
		executor:    executor,
		queue:       make(chan string, 128),
		log:         log,
	}
}

func (p *Pipeline) Start(ctx context.Context, workers int) {
	if workers <= 0 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		go p.worker(ctx)
	}
}

func (p *Pipeline) Enqueue(taskID string) {
	p.queue <- taskID
}

func (p *Pipeline) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case taskID := <-p.queue:
			if err := p.process(ctx, taskID); err != nil {
				p.log.Error("task processing failed", "task_id", taskID, "error", err)
			}
		}
	}
}

func (p *Pipeline) process(ctx context.Context, taskID string) error {
	task, err := p.tasks.FindByID(taskID)
	if err != nil {
		return err
	}

	task = p.mark(task, core.TaskStatusTranscribing, "")
	text := task.OriginalText
	if text == "" {
		text, err = p.transcriber.Transcribe(ctx, task.AudioPath)
		if err != nil {
			p.fail(task, err)
			return err
		}
	}

	task.OriginalText = text
	task = p.mark(task, core.TaskStatusParsing, "")
	command, err := p.parser.Parse(ctx, text)
	if err != nil {
		p.fail(task, err)
		return err
	}

	task.ParsedCommand = command
	task = p.mark(task, core.TaskStatusExecutingCAD, "")
	resultPath, err := p.executor.Execute(ctx, task)
	if err != nil {
		p.fail(task, err)
		return err
	}

	now := time.Now().UTC()
	task.Status = core.TaskStatusDone
	task.ResultPath = resultPath
	task.ResultAvailable = true
	task.UpdatedAt = now
	task.CompletedAt = &now
	return p.tasks.Update(task)
}

func (p *Pipeline) mark(task core.Task, status core.TaskStatus, message string) core.Task {
	task.Status = status
	task.Error = message
	task.UpdatedAt = time.Now().UTC()
	if err := p.tasks.Update(task); err != nil {
		p.log.Error("task status update failed", "task_id", task.ID, "error", err)
	}
	return task
}

func (p *Pipeline) fail(task core.Task, cause error) {
	task.Status = core.TaskStatusFailed
	task.Error = cause.Error()
	task.UpdatedAt = time.Now().UTC()
	_ = p.tasks.Update(task)
}

type HTTPTranscriber struct {
	URL string
}

func (c HTTPTranscriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	if c.URL == "" {
		return "create rectangular plate 100 by 50 with thickness 5 mm", nil
	}

	body, _ := json.Marshal(map[string]string{"audio_path": audioPath})
	var response struct {
		Text string `json:"text"`
	}
	if err := postJSON(ctx, c.URL, body, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Text) == "" {
		return "", fmt.Errorf("transcriber returned empty text")
	}
	return response.Text, nil
}

type HTTPParser struct {
	URL string
}

func (c HTTPParser) Parse(ctx context.Context, text string) (core.Command, error) {
	if c.URL == "" {
		return parseDemoCommand(text), nil
	}

	body, _ := json.Marshal(map[string]string{"text": text})
	var response core.Command
	if err := postJSON(ctx, c.URL, body, &response); err != nil {
		return core.Command{}, err
	}
	if response.Intent == "" {
		return core.Command{}, fmt.Errorf("parser returned empty command")
	}
	return response, nil
}

type HTTPCADExecutor struct {
	URL     string
	Files   *storage.LocalStorage
	Timeout time.Duration
}

func (c HTTPCADExecutor) Execute(ctx context.Context, task core.Task) (string, error) {
	if c.URL == "" {
		content := fmt.Sprintf("VoiceCAD demo result\nTask: %s\nPlatform: %s\nText: %s\nCommand: %+v\n", task.ID, task.CADPlatform, task.OriginalText, task.ParsedCommand)
		return c.Files.CreateText("results", task.ID+".txt", content)
	}

	body, _ := json.Marshal(map[string]any{
		"task_id":      task.ID,
		"cad_platform": task.CADPlatform,
		"command":      task.ParsedCommand,
	})
	var response struct {
		ResultPath string `json:"result_path"`
	}
	if err := postJSON(ctx, c.URL, body, &response); err != nil {
		return "", err
	}
	if response.ResultPath == "" {
		return "", fmt.Errorf("cad executor returned empty result_path")
	}
	if _, err := os.Stat(response.ResultPath); err != nil {
		return "", err
	}
	return response.ResultPath, nil
}

func postJSON(ctx context.Context, url string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("service returned status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func parseDemoCommand(text string) core.Command {
	lower := strings.ToLower(text)
	primitive := "plate"
	if strings.Contains(lower, "cylinder") {
		primitive = "cylinder"
	}
	if strings.Contains(lower, "cube") || strings.Contains(lower, "box") {
		primitive = "box"
	}

	return core.Command{
		Intent:    "create_part",
		Primitive: primitive,
		Units:     "mm",
		Parameters: map[string]any{
			"source_text": text,
		},
	}
}
