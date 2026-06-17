package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/hydroan/gst/types"
)

// Session is deprecated
type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	SessionID    string `json:"session_id"`

	// TODO: 统一起来，使用 model.UserAgent
	Platform       string `json:"platform"`
	OS             string `json:"os"`
	EngineName     string `json:"engine_name"`
	EngineVersion  string `json:"engine_version"`
	BrowserName    string `json:"browser_name"`
	BrowserVersion string `json:"browser_version"`

	Base
}

func (s *Session) initDefault() error {
	s.ID = s.id()
	return nil
}

func (s *Session) CreateBefore(*types.ModelContext) error { return s.initDefault() }
func (s *Session) UpdateBefore(*types.ModelContext) error { return s.initDefault() }
func (s *Session) DeleteBefore(*types.ModelContext) error {
	s.ID = s.id()
	return nil
}

func (s *Session) id() string {
	parts := []string{
		s.UserID,
		s.Platform,
		s.OS,
		s.EngineName,
		s.BrowserName,
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(hash[:8])
}
