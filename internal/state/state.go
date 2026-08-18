package state

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ThreadEntry tracks the last backed-up fingerprint of a conversation.
type ThreadEntry struct {
	Name        string    `yaml:"name"`
	LastSnippet string    `yaml:"last_snippet"`
	LastTime    string    `yaml:"last_time"`
	BackedUpAt  time.Time `yaml:"backed_up_at"`
}

// State is stored as backup_state.yaml inside the download directory.
type State struct {
	Threads map[string]ThreadEntry `yaml:"threads"`
	// NameIndex maps participant name → thread id for pre-first-backup matching.
	ByName map[string]string `yaml:"by_name,omitempty"`
}

func pathIn(downloadDir string) string {
	return filepath.Join(downloadDir, "backup_state.yaml")
}

// Load reads backup_state.yaml from the download directory.
func Load(downloadDir string) (*State, error) {
	data, err := os.ReadFile(pathIn(downloadDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Threads: map[string]ThreadEntry{}, ByName: map[string]string{}}, nil
		}
		return nil, err
	}
	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Threads == nil {
		s.Threads = map[string]ThreadEntry{}
	}
	if s.ByName == nil {
		s.ByName = map[string]string{}
	}
	return &s, nil
}

// Save writes backup_state.yaml.
func Save(downloadDir string, s *State) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(pathIn(downloadDir), data, 0o644)
}

// Fingerprint identifies the latest inbox row for a conversation.
func Fingerprint(snippet, timeStamp string) string {
	return snippet + "\n" + timeStamp
}

// IsBackedUp reports whether the conversation matches a saved fingerprint.
// Prefer threadID when known; otherwise match by participant name.
func (s *State) IsBackedUp(threadID, name, snippet, timeStamp string) bool {
	fp := Fingerprint(snippet, timeStamp)
	if threadID != "" {
		if e, ok := s.Threads[threadID]; ok {
			return Fingerprint(e.LastSnippet, e.LastTime) == fp
		}
	}
	if id, ok := s.ByName[name]; ok {
		if e, ok := s.Threads[id]; ok {
			return Fingerprint(e.LastSnippet, e.LastTime) == fp
		}
	}
	// Also try name-keyed entries if ByName is empty (older state).
	for _, e := range s.Threads {
		if e.Name == name {
			return Fingerprint(e.LastSnippet, e.LastTime) == fp
		}
	}
	return false
}

// Record records a successful backup.
func (s *State) Record(threadID, name, snippet, timeStamp string) {
	if s.Threads == nil {
		s.Threads = map[string]ThreadEntry{}
	}
	if s.ByName == nil {
		s.ByName = map[string]string{}
	}
	s.Threads[threadID] = ThreadEntry{
		Name:        name,
		LastSnippet: snippet,
		LastTime:    timeStamp,
		BackedUpAt:  time.Now().UTC(),
	}
	if name != "" {
		s.ByName[name] = threadID
	}
}
