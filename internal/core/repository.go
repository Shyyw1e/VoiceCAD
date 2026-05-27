package core

import (
	"errors"
	"sync"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

type UserRepository interface {
	Create(user User) error
	FindByEmail(email string) (User, error)
	FindByID(id string) (User, error)
}

type TaskRepository interface {
	Create(task Task) error
	Update(task Task) error
	FindByID(id string) (Task, error)
	ListByUser(userID string) ([]Task, error)
}

type MemoryUserRepository struct {
	mu      sync.RWMutex
	byID    map[string]User
	byEmail map[string]string
}

func NewMemoryUserRepository() *MemoryUserRepository {
	return &MemoryUserRepository{
		byID:    make(map[string]User),
		byEmail: make(map[string]string),
	}
}

func (r *MemoryUserRepository) Create(user User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byEmail[user.Email]; exists {
		return ErrAlreadyExists
	}

	r.byID[user.ID] = user
	r.byEmail[user.Email] = user.ID
	return nil
}

func (r *MemoryUserRepository) FindByEmail(email string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, exists := r.byEmail[email]
	if !exists {
		return User{}, ErrNotFound
	}

	return r.byID[id], nil
}

func (r *MemoryUserRepository) FindByID(id string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.byID[id]
	if !exists {
		return User{}, ErrNotFound
	}

	return user, nil
}

type MemoryTaskRepository struct {
	mu   sync.RWMutex
	byID map[string]Task
}

func NewMemoryTaskRepository() *MemoryTaskRepository {
	return &MemoryTaskRepository{byID: make(map[string]Task)}
}

func (r *MemoryTaskRepository) Create(task Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[task.ID]; exists {
		return ErrAlreadyExists
	}

	r.byID[task.ID] = task
	return nil
}

func (r *MemoryTaskRepository) Update(task Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[task.ID]; !exists {
		return ErrNotFound
	}

	r.byID[task.ID] = task
	return nil
}

func (r *MemoryTaskRepository) FindByID(id string) (Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, exists := r.byID[id]
	if !exists {
		return Task{}, ErrNotFound
	}

	return task, nil
}

func (r *MemoryTaskRepository) ListByUser(userID string) ([]Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tasks := make([]Task, 0)
	for _, task := range r.byID {
		if task.UserID == userID {
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
}
