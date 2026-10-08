package session

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Queue keeps tasks for users who are not logged in, in a file so they
// survive a daemon restart.
type Queue struct {
	path string
	mu   sync.Mutex
}

type queued struct {
	UID  int             `json:"uid"`
	Task string          `json:"task"`
	Args json.RawMessage `json:"args,omitempty"`
}

// NewQueue keeps its items in path (written through a temp file, 0600).
func NewQueue(path string) *Queue { return &Queue{path: path} }

func (q *Queue) load() ([]queued, error) {
	b, err := os.ReadFile(q.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []queued
	return items, json.Unmarshal(b, &items)
}

func (q *Queue) save(items []queued) error {
	if len(items) == 0 {
		if err := os.Remove(q.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(q.path), 0o700); err != nil {
		return err
	}
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, q.path)
}

// Add queues task for uid.
func (q *Queue) Add(uid int, task string, args any) error {
	raw, err := json.Marshal(args)
	if err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	items, err := q.load()
	if err != nil {
		return err
	}
	return q.save(append(items, queued{UID: uid, Task: task, Args: raw}))
}

// Drain runs u's items in order; the ones run fails on stay queued.
func (q *Queue) Drain(u User, run func(task string, args json.RawMessage) error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	items, err := q.load()
	if err != nil {
		return err
	}
	var keep []queued
	var errs []error
	for _, it := range items {
		if it.UID != u.UID {
			keep = append(keep, it)
			continue
		}
		if err := run(it.Task, it.Args); err != nil {
			errs = append(errs, err)
			keep = append(keep, it)
		}
	}
	return errors.Join(append(errs, q.save(keep))...)
}
