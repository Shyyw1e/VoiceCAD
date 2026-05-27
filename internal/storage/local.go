package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) (*LocalStorage, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	return &LocalStorage{root: root}, nil
}

func (s *LocalStorage) Save(category, filename string, src io.Reader) (string, error) {
	dir := filepath.Join(s.root, cleanPathPart(category))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, cleanPathPart(filename))
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, src); err != nil {
		return "", err
	}

	return path, nil
}

func (s *LocalStorage) CreateText(category, filename, content string) (string, error) {
	return s.Save(category, filename, strings.NewReader(content))
}

func (s *LocalStorage) Open(path string) (*os.File, error) {
	cleanRoot, err := filepath.Abs(s.root)
	if err != nil {
		return nil, err
	}

	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(cleanPath, cleanRoot) {
		return nil, fmt.Errorf("path escapes storage root")
	}

	return os.Open(cleanPath)
}

func cleanPathPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "..", "_")
	if value == "" {
		return "file"
	}
	return value
}
