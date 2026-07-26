// Package memory gives the gateway conversation memory, so a project gets
// history without building a store of its own.
//
// A conversation belongs to exactly one project. That is the load-bearing
// property: putting every project's messages in one service means a bug in
// scoping is a cross-tenant data leak, so the project is part of the path, is
// re-checked on every read, and a conversation id is never trusted as a
// filename.
//
// One JSON file per conversation under <root>/<project>/<id>.json. No database:
// a conversation is small and bounded, reads and writes are per-conversation
// rather than global, and the alternative is a second thing to deploy, back up
// and fail. Writes are atomic (temp + rename).
package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrNotFound  = errors.New("conversation not found")
	ErrBadID     = errors.New("invalid conversation id")
	ErrNoProject = errors.New("conversation memory requires a project-scoped key")
)

// Limits on what is kept. History that grows without bound eventually exceeds
// the model's context and the request starts failing with no obvious cause, so
// the store trims rather than letting that happen.
const (
	// MaxTurns is how many messages are retained per conversation.
	MaxTurns = 40
	// MaxChars bounds the total characters replayed into a prompt.
	MaxChars = 24000
	// MaxIDLen bounds a client-supplied id.
	MaxIDLen = 128
)

// Message is one stored turn.
type Message struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// Conversation is the stored history for one id, within one project.
type Conversation struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store persists conversations under a root directory.
type Store struct {
	root string
	mu   sync.Mutex // serialises read-modify-write on a conversation file
}

func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("memory: create %s: %w", root, err)
	}
	return &Store{root: root}, nil
}

// safeID rejects anything that could escape the project directory or collide
// with another project's file. Ids come from clients, so this is the boundary
// that keeps a conversation id from being a path.
func safeID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > MaxIDLen {
		return "", ErrBadID
	}
	for _, r := range id {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
		if !ok {
			return "", ErrBadID
		}
	}
	return id, nil
}

func safeProject(project string) (string, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return "", ErrNoProject
	}
	// Same rule as an id: a project name reaches the filesystem too.
	return safeID(project)
}

func (s *Store) path(project, id string) (string, error) {
	p, err := safeProject(project)
	if err != nil {
		return "", err
	}
	i, err := safeID(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, p, i+".json"), nil
}

// Load returns a conversation, or ErrNotFound.
//
// The project is part of the path, so asking for another project's id simply
// looks in the wrong directory and finds nothing — there is no code path where
// a match in one project can be returned to another.
func (s *Store) Load(project, id string) (*Conversation, error) {
	path, err := s.path(project, id)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var c Conversation
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("memory: parse %s: %w", path, err)
	}
	// Belt and braces: if a file somehow carries another project's name, refuse
	// it rather than serve it.
	if c.Project != "" && c.Project != project {
		return nil, ErrNotFound
	}
	return &c, nil
}

// Append adds messages to a conversation, creating it if needed, and trims it
// back within the retention limits.
func (s *Store) Append(project, id string, msgs ...Message) (*Conversation, error) {
	path, err := s.path(project, id)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	c := &Conversation{ID: id, Project: project, CreatedAt: now}

	if raw, err := os.ReadFile(path); err == nil {
		var existing Conversation
		if err := json.Unmarshal(raw, &existing); err == nil && existing.Project == project {
			c = &existing
		}
	}

	for _, m := range msgs {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		if m.At.IsZero() {
			m.At = now
		}
		c.Messages = append(c.Messages, m)
	}
	c.UpdatedAt = now
	trim(c)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return c, nil
}

// trim keeps the tail of a conversation within both limits.
//
// The tail, not the head: the recent turns are what the next reply depends on.
// A summary of the dropped prefix would be better and is a deliberate
// non-goal for now — it needs a model call on the write path, which would make
// every stored turn cost money.
func trim(c *Conversation) {
	if len(c.Messages) > MaxTurns {
		c.Messages = c.Messages[len(c.Messages)-MaxTurns:]
	}
	total := 0
	for i := len(c.Messages) - 1; i >= 0; i-- {
		total += len(c.Messages[i].Content)
		if total > MaxChars {
			c.Messages = c.Messages[i+1:]
			return
		}
	}
}

// List returns the project's conversations, most recently updated first.
func (s *Store) List(project string) ([]Conversation, error) {
	p, err := safeProject(project)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(filepath.Join(s.root, p))
	if errors.Is(err, os.ErrNotExist) {
		return []Conversation{}, nil
	}
	if err != nil {
		return nil, err
	}

	out := make([]Conversation, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.root, p, e.Name()))
		if err != nil {
			continue
		}
		var c Conversation
		if err := json.Unmarshal(raw, &c); err != nil || c.Project != project {
			continue
		}
		// Summaries only: a list endpoint returning every message of every
		// conversation would ship the project's entire history on one call.
		c.Messages = nil
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Delete removes one conversation.
func (s *Store) Delete(project, id string) error {
	path, err := s.path(project, id)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// Stats reports how much a project is storing, for the console.
func (s *Store) Stats(project string) (conversations, messages int) {
	list, err := s.List(project)
	if err != nil {
		return 0, 0
	}
	for _, c := range list {
		full, err := s.Load(project, c.ID)
		if err != nil {
			continue
		}
		messages += len(full.Messages)
	}
	return len(list), messages
}

// Projects returns the projects that have stored anything.
func (s *Store) Projects() []string {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}
