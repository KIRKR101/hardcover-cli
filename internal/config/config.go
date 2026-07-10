// Package config handles reading and writing the Hardcover CLI
// configuration file (a JSON document containing the API token).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/term"

	"github.com/KIRKR101/hardcover-cli/internal/errs"
)

// FileName is the file used in the user's home directory.
const FileName = ".hardcover.json"

// EnvVar is the fallback environment variable for the API token.
const EnvVar = "HARDCOVER_TOKEN"

// path returns the absolute path to the config file.
func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, FileName), nil
}

// fileMode is 0600: read/write for owner only. The file contains
// a bearer token, so other users on the machine must not read it.
const fileMode os.FileMode = 0o600

// configFile is the JSON shape on disk.
type configFile struct {
	Token string `json:"token"`
}

// LoadToken returns the API token from the config file, or the
// HARDCOVER_TOKEN env var as a fallback. Returns errs.ErrNoToken
// (wrapped) if neither yields a value.
func LoadToken() (string, error) {
	p, err := path()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Fall through to env var.
	case err != nil:
		return "", fmt.Errorf("read %s: %w", p, err)
	default:
		var c configFile
		if err := json.Unmarshal(data, &c); err != nil {
			return "", fmt.Errorf("parse %s: %w", p, err)
		}
		if c.Token != "" {
			return c.Token, nil
		}
	}
	if t := os.Getenv(EnvVar); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("run `hardcover setup` first: %w", errs.ErrNoToken)
}

// SaveToken writes the token to the config file with 0600 permissions.
// On non-Windows systems, explicitly chmods the file in case it already
// existed with looser permissions.
func SaveToken(token string) error {
	p, err := path()
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("refusing to save empty token: %w", errs.ErrInvalid)
	}
	data, err := json.MarshalIndent(configFile{Token: token}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	// 0600 explicitly. We do not use os.WriteFile (which uses 0666 & umask).
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("create %s: %w", p, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", p, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", p, err)
	}
	// Defensive: if the file already existed with looser perms, tighten.
	_ = os.Chmod(p, fileMode)
	return nil
}

// PromptToken reads a token from the terminal with echo disabled,
// falling back to plain read if the terminal doesn't support raw mode.
func PromptToken() (string, error) {
	fmt.Print("Paste your Hardcover API token: ")
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("read token: %w", err)
		}
		return string(b), nil
	}
	// Not a TTY; read plain.
	var buf [4096]byte
	n, err := os.Stdin.Read(buf[:])
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	return string(buf[:n]), nil
}
