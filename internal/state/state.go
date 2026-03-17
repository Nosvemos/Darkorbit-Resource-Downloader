package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const FileName = ".app-state.json"

type State struct {
	UpdatedAt string                `json:"updated_at"`
	Resources map[string]StateEntry `json:"resources"`
}

type StateEntry struct {
	Hash string `json:"hash,omitempty"`
	URL  string `json:"url,omitempty"`
}

func Load(outputDir string) (*State, error) {
	filePath := filepath.Join(outputDir, FileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Resources: map[string]StateEntry{}}, nil
		}
		return nil, err
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.Resources == nil {
		st.Resources = map[string]StateEntry{}
	}
	return &st, nil
}

func Save(outputDir string, st *State) error {
	if st.Resources == nil {
		st.Resources = map[string]StateEntry{}
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	filePath := filepath.Join(outputDir, FileName)
	return os.WriteFile(filePath, data, 0o644)
}
