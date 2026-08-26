package project

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const projectConfRelativePath = ".agents/loaf.conf"

// ProjectConf is the conf-carried rendezvous label shipped with the clone.
// It selects the project within an operator substrate; it never carries sync endpoints.
type ProjectConf struct {
	ConfID    string `json:"conf_id"`
	ProjectID string `json:"project_id"`
}

// ReadProjectConf loads `.agents/loaf.conf` from the canonical project root.
func ReadProjectConf(root Root) (ProjectConf, error) {
	path := filepath.Join(root.Path(), projectConfRelativePath)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectConf{}, fmt.Errorf("project conf missing at %s", path)
		}
		return ProjectConf{}, fmt.Errorf("read project conf: %w", err)
	}
	var conf ProjectConf
	if err := json.Unmarshal(raw, &conf); err != nil {
		return ProjectConf{}, fmt.Errorf("decode project conf: %w", err)
	}
	conf.ConfID = strings.TrimSpace(conf.ConfID)
	conf.ProjectID = strings.TrimSpace(conf.ProjectID)
	if conf.ProjectID == "" {
		return ProjectConf{}, fmt.Errorf("project conf %s: project_id is required", path)
	}
	return conf, nil
}

// WriteProjectConf writes `.agents/loaf.conf` under the project root.
func WriteProjectConf(root Root, conf ProjectConf) error {
	conf.ConfID = strings.TrimSpace(conf.ConfID)
	conf.ProjectID = strings.TrimSpace(conf.ProjectID)
	if conf.ProjectID == "" {
		return fmt.Errorf("write project conf: project_id is required")
	}
	if conf.ConfID == "" {
		id, err := GenerateConfID()
		if err != nil {
			return err
		}
		conf.ConfID = id
	}
	path := filepath.Join(root.Path(), projectConfRelativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create agents directory: %w", err)
	}
	raw, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project conf: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write project conf: %w", err)
	}
	return nil
}

// GenerateConfID mints a time-ordered rendezvous label for `.agents/loaf.conf`.
func GenerateConfID() (string, error) {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate conf id: %w", err)
	}
	ms := time.Now().UTC().UnixMilli()
	return fmt.Sprintf("conf_%x_%s", ms, hex.EncodeToString(entropy[:])), nil
}
