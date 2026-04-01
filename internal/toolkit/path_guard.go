package toolkit

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func resolveSafePath(root, target string) (string, error) {
	if target == "" {
		target = "."
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root path: %w", err)
	}
	var joined string
	if filepath.IsAbs(target) {
		joined = filepath.Clean(target)
	} else {
		joined = filepath.Join(cleanRoot, target)
	}
	absTarget, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("resolve target path: %w", err)
	}
	rel, err := filepath.Rel(cleanRoot, absTarget)
	if err != nil {
		return "", fmt.Errorf("calculate relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path is outside WORKSPACE_ROOT")
	}
	return absTarget, nil
}
