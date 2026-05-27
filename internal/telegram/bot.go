package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Shyyw1e/VoiceCAD/internal/core"
	"github.com/Shyyw1e/VoiceCAD/internal/pipeline"
	"github.com/Shyyw1e/VoiceCAD/internal/storage"
)

type Bot struct {
	client   *Client
	users    core.UserRepository
	tasks    core.TaskRepository
	files    *storage.LocalStorage
	pipeline *pipeline.Pipeline
	log      *slog.Logger
}

func NewBot(client *Client, users core.UserRepository, tasks core.TaskRepository, files *storage.LocalStorage, pipe *pipeline.Pipeline, log *slog.Logger) *Bot {
	return &Bot{
		client:   client,
		users:    users,
		tasks:    tasks,
		files:    files,
		pipeline: pipe,
		log:      log,
	}
}

func (b *Bot) Start(ctx context.Context) {
	go b.poll(ctx)
}

func (b *Bot) poll(ctx context.Context) {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := b.client.GetUpdates(ctx, offset, 25)
		if err != nil {
			b.log.Error("telegram updates failed", "error", err)
			sleep(ctx, 2*time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.ID + 1
			if update.Message == nil {
				continue
			}
			b.handleMessage(ctx, *update.Message)
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, message Message) {
	text := strings.TrimSpace(message.Text)
	if strings.HasPrefix(text, "/start") || strings.HasPrefix(text, "/help") {
		_ = b.client.SendMessage(ctx, message.Chat.ID, "VoiceCAD MVP is ready. Send a text command, voice message, or audio file. Example: create rectangular plate 100 by 50 with thickness 5 mm")
		return
	}

	user, err := b.ensureUser(message)
	if err != nil {
		b.log.Error("telegram user ensure failed", "chat_id", message.Chat.ID, "error", err)
		_ = b.client.SendMessage(ctx, message.Chat.ID, "Could not prepare a user for this task.")
		return
	}

	task, err := b.createTask(ctx, user, message)
	if err != nil {
		b.log.Error("telegram task create failed", "chat_id", message.Chat.ID, "error", err)
		_ = b.client.SendMessage(ctx, message.Chat.ID, "Could not create task: "+err.Error())
		return
	}

	_ = b.client.SendMessage(ctx, message.Chat.ID, "Task created: "+task.ID+". Processing.")
	b.pipeline.Enqueue(task.ID)
	go b.watchTask(ctx, message.Chat.ID, task.ID)
}

func (b *Bot) ensureUser(message Message) (core.User, error) {
	email := fmt.Sprintf("telegram_%d@voicecad.local", message.Chat.ID)
	if user, err := b.users.FindByEmail(email); err == nil {
		return user, nil
	}

	name := "Telegram user"
	if message.From != nil {
		name = strings.TrimSpace(message.From.FirstName + " " + message.From.LastName)
		if name == "" && message.From.Username != "" {
			name = message.From.Username
		}
	}

	passwordHash, err := core.HashPassword("telegram-" + strconv.FormatInt(message.Chat.ID, 10))
	if err != nil {
		return core.User{}, err
	}

	user := core.User{
		ID:           core.NewID("usr"),
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
	}
	if err := b.users.Create(user); err != nil {
		return core.User{}, err
	}
	return user, nil
}

func (b *Bot) createTask(ctx context.Context, user core.User, message Message) (core.Task, error) {
	taskID := core.NewID("tsk")
	now := time.Now().UTC()
	task := core.Task{
		ID:          taskID,
		UserID:      user.ID,
		Status:      core.TaskStatusQueued,
		CADPlatform: core.CADPlatformKompas,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if text := strings.TrimSpace(message.Text); text != "" {
		task.OriginalText = text
		return task, b.tasks.Create(task)
	}

	fileID, filename, err := messageFile(message)
	if err != nil {
		return core.Task{}, err
	}

	telegramFile, err := b.client.GetFile(ctx, fileID)
	if err != nil {
		return core.Task{}, err
	}

	reader, writer := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := b.client.DownloadFile(ctx, telegramFile.FilePath, writer)
		_ = writer.CloseWithError(err)
		errCh <- err
	}()

	task.AudioPath, err = b.files.Save("audio", taskID+"_"+filename, reader)
	if err != nil {
		_ = reader.Close()
		return core.Task{}, err
	}
	if err := <-errCh; err != nil {
		return core.Task{}, err
	}

	return task, b.tasks.Create(task)
}

func (b *Bot) watchTask(ctx context.Context, chatID int64, taskID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.NewTimer(3 * time.Minute)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout.C:
			_ = b.client.SendMessage(ctx, chatID, "Task "+taskID+" is still processing. You can check its status later through the HTTP API.")
			return
		case <-ticker.C:
			task, err := b.tasks.FindByID(taskID)
			if err != nil {
				_ = b.client.SendMessage(ctx, chatID, "Task "+taskID+" was not found.")
				return
			}
			switch task.Status {
			case core.TaskStatusDone:
				if err := b.client.SendDocument(ctx, chatID, task.ResultPath, "Done: "+task.ID); err != nil {
					b.log.Error("telegram send result failed", "task_id", taskID, "error", err)
					_ = b.client.SendMessage(ctx, chatID, "Task is done, but I could not send the result file.")
				}
				return
			case core.TaskStatusFailed:
				_ = b.client.SendMessage(ctx, chatID, "Task "+task.ID+" failed: "+task.Error)
				return
			}
		}
	}
}

func messageFile(message Message) (fileID, filename string, err error) {
	if message.Voice != nil {
		return message.Voice.FileID, message.Voice.FileUniqueID + ".ogg", nil
	}
	if message.Audio != nil {
		name := message.Audio.FileName
		if name == "" {
			name = message.Audio.FileUniqueID + ".audio"
		}
		return message.Audio.FileID, filepath.Base(name), nil
	}
	if message.Document != nil {
		name := message.Document.FileName
		if name == "" {
			name = message.Document.FileUniqueID
		}
		return message.Document.FileID, filepath.Base(name), nil
	}
	return "", "", fmt.Errorf("send text, a voice message, or an audio file")
}

func sleep(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
