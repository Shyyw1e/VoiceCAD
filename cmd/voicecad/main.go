package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shyyw1e/VoiceCAD/internal/api"
	"github.com/Shyyw1e/VoiceCAD/internal/config"
	"github.com/Shyyw1e/VoiceCAD/internal/pipeline"
	"github.com/Shyyw1e/VoiceCAD/internal/postgres"
	"github.com/Shyyw1e/VoiceCAD/internal/storage"
	tgbot "github.com/Shyyw1e/VoiceCAD/internal/telegram"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	log := newLogger(cfg.Log)
	files, err := storage.NewLocalStorage(cfg.Storage.LocalDir)
	if err != nil {
		log.Error("storage init failed", "error", err)
		os.Exit(1)
	}

	db, err := postgres.Open(context.Background(), cfg.Postgres)
	if err != nil {
		log.Error("postgres connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := postgres.Migrate(context.Background(), db); err != nil {
		log.Error("postgres migrations failed", "error", err)
		os.Exit(1)
	}

	users := postgres.NewUserRepository(db)
	tasks := postgres.NewTaskRepository(db)
	pipe := pipeline.New(
		tasks,
		files,
		pipeline.HTTPTranscriber{URL: cfg.ML.TranscriberURL},
		pipeline.HTTPParser{URL: cfg.ML.ParserURL},
		pipeline.HTTPCADExecutor{URL: cfg.CAD.ExecutorURL, Files: files, Timeout: 2 * time.Minute},
		log,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pipe.Start(ctx, 2)
	if cfg.Telegram.BotToken != "" {
		bot := tgbot.NewBot(tgbot.NewClient(cfg.Telegram.BotToken), users, tasks, files, pipe, log)
		bot.Start(ctx)
		log.Info("telegram bot started")
	}

	handler := api.NewServer(users, tasks, files, pipe, log).Routes()
	server := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      handler,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	go func() {
		log.Info("voicecad backend started", "addr", cfg.HTTP.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown failed", "error", err)
	}
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Level == "debug" {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{Level: level}
	if cfg.Format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
