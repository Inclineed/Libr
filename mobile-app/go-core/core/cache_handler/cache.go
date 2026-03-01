package cache

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libr-forum/Libr/core/crypto/config"
	"github.com/libr-forum/Libr/core/mod_client/logger"
	"github.com/libr-forum/Libr/core/mod_client/types"
)

// GetCacheDir returns the directory used for caching pending moderations.
// On mobile, we use the same directory configured for keys.
func GetCacheDir() string {
	return filepath.Dir(config.PrivateKeyPath)
}

// SavePendingModeration saves a PendingModeration object to disk.
func SavePendingModeration(pending types.PendingModeration) error {
	dir := filepath.Join(GetCacheDir(), "pending_mods")
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.LogToFile("[DEBUG] Failed to create pending_mods dir")
		return fmt.Errorf("failed to create pending_mods dir: %w", err)
	}

	filePath := filepath.Join(dir, sanitizeFileName(pending.MsgSign)+".json")
	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		logger.LogToFile("[DEBUG] Failed to marshal pending moderation")
		return fmt.Errorf("failed to marshal pending moderation: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write pending file: %w", err)
	}

	return nil
}

// LoadPendingModeration loads a PendingModeration object from a specific file path.
func LoadPendingModeration(filePath string) (types.PendingModeration, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return types.PendingModeration{}, fmt.Errorf("failed to read pending file: %w", err)
	}

	var pending types.PendingModeration
	if err := json.Unmarshal(data, &pending); err != nil {
		return types.PendingModeration{}, fmt.Errorf("failed to unmarshal pending data: %w", err)
	}

	return pending, nil
}

// DeletePendingModeration deletes a pending moderation file by its message signature.
func DeletePendingModeration(msgSign string) error {
	dir := filepath.Join(GetCacheDir(), "pending_mods")
	filePath := filepath.Join(dir, sanitizeFileName(msgSign)+".json")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete pending moderation file: %w", err)
	}
	return nil
}

// GetAllPendingModerations returns a list of all currently pending moderations.
func GetAllPendingModerations() ([]types.PendingModeration, error) {
	pattern := filepath.Join(GetCacheDir(), "pending_mods", "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending files: %w", err)
	}

	var results []types.PendingModeration
	for _, filePath := range files {
		if pending, err := LoadPendingModeration(filePath); err == nil {
			results = append(results, pending)
		}
	}
	return results, nil
}

func sanitizeFileName(msgSign string) string {
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(msgSign))
}
