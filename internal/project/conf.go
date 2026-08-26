package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
