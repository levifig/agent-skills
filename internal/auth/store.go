package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	attachStateFileName = "attach.json"
	adminWireFileName   = "admin.wire"
	localConfigFileName = "local.json"
)

// AttachState records whether this machine environment completed attach.
type AttachState struct {
	Attached       bool   `json:"attached"`
	Endpoint       string `json:"endpoint,omitempty"`
	ConnectionName string `json:"connection_name,omitempty"`
	AttachedAt     string `json:"attached_at,omitempty"`
}

// LocalConfig holds non-wire local auth configuration (self-host paths).
type LocalConfig struct {
	ServerDB string `json:"server_db"`
}

// Store persists machine-local auth state under the Loaf state home.
type Store struct {
	Dir string
}

// NewStore returns the auth store rooted at stateHome/auth.
func NewStore(authDir string) Store {
	return Store{Dir: filepath.Clean(strings.TrimSpace(authDir))}
}

func (s Store) ensureDir() error {
	if strings.TrimSpace(s.Dir) == "" {
		return errors.New("auth store directory is unset")
	}
	return os.MkdirAll(s.Dir, 0o700)
}

func (s Store) attachStatePath() string {
	return filepath.Join(s.Dir, attachStateFileName)
}

func (s Store) adminWirePath() string {
	return filepath.Join(s.Dir, adminWireFileName)
}

func (s Store) localConfigPath() string {
	return filepath.Join(s.Dir, localConfigFileName)
}

// LoadAttachState reads persisted attach state. Missing file means unattached.
func (s Store) LoadAttachState() (AttachState, error) {
	raw, err := os.ReadFile(s.attachStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return AttachState{}, nil
		}
		return AttachState{}, fmt.Errorf("read attach state: %w", err)
	}
	var state AttachState
	if err := json.Unmarshal(raw, &state); err != nil {
		return AttachState{}, fmt.Errorf("decode attach state: %w", err)
	}
	return state, nil
}

// SaveAttachState writes attach state atomically.
func (s Store) SaveAttachState(state AttachState) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode attach state: %w", err)
	}
	tmp := s.attachStatePath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write attach state: %w", err)
	}
	if err := os.Rename(tmp, s.attachStatePath()); err != nil {
		return fmt.Errorf("commit attach state: %w", err)
	}
	return nil
}

// IsAttached reports whether the environment completed attach.
func (s Store) IsAttached() (bool, error) {
	state, err := s.LoadAttachState()
	if err != nil {
		return false, err
	}
	return state.Attached, nil
}

// LoadAdminWire returns the encoded admin wire string when present.
func (s Store) LoadAdminWire() (string, error) {
	raw, err := os.ReadFile(s.adminWirePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrAdminNotConfigured
		}
		return "", fmt.Errorf("read admin wire: %w", err)
	}
	wire := strings.TrimSpace(string(raw))
	if wire == "" {
		return "", ErrAdminNotConfigured
	}
	return wire, nil
}

// SaveAdminWire stores the encoded admin wire string.
func (s Store) SaveAdminWire(wire string) error {
	wire = strings.TrimSpace(wire)
	if wire == "" {
		return errors.New("admin wire is empty")
	}
	if err := s.ensureDir(); err != nil {
		return err
	}
	tmp := s.adminWirePath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(wire+"\n"), 0o600); err != nil {
		return fmt.Errorf("write admin wire: %w", err)
	}
	return os.Rename(tmp, s.adminWirePath())
}

// LoadLocalConfig reads self-host sync server DB path configuration.
func (s Store) LoadLocalConfig() (LocalConfig, error) {
	raw, err := os.ReadFile(s.localConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return LocalConfig{}, nil
		}
		return LocalConfig{}, fmt.Errorf("read local auth config: %w", err)
	}
	var cfg LocalConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return LocalConfig{}, fmt.Errorf("decode local auth config: %w", err)
	}
	return cfg, nil
}

// SaveLocalConfig writes self-host sync server DB path configuration.
func (s Store) SaveLocalConfig(cfg LocalConfig) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode local auth config: %w", err)
	}
	tmp := s.localConfigPath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write local auth config: %w", err)
	}
	return os.Rename(tmp, s.localConfigPath())
}
// EnforcementActive reports whether attach refusal should gate substrate commands.
// Gate engages once auth setup created admin wire or attach state exists on this machine.
func (s Store) EnforcementActive() (bool, error) {
	if _, err := os.Stat(s.adminWirePath()); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if _, err := os.Stat(s.attachStatePath()); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}

