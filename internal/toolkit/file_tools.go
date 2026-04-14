package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

type fileToolSet struct {
	workspaceRoot string
	policy        Policy
}

type listDirInput struct {
	Path string `json:"path"`
}
type listDirOutput struct {
	Path    string   `json:"path"`
	Exists  bool     `json:"exists"`
	Entries []string `json:"entries"`
}

func (f *fileToolSet) listDir(_ context.Context, in listDirInput) (*listDirOutput, error) {
	target, err := resolveSafePath(f.workspaceRoot, in.Path)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(target)
	if err != nil {
		if os.IsNotExist(err) {
			return &listDirOutput{Path: target, Exists: false, Entries: []string{}}, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}
	out := &listDirOutput{Path: target, Exists: true, Entries: make([]string, 0, len(items))}
	for _, item := range items {
		name := item.Name()
		if item.IsDir() {
			name += "/"
		}
		out.Entries = append(out.Entries, name)
	}
	return out, nil
}

type readFileInput struct {
	Path string `json:"path"`
}
type readFileOutput struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Content string `json:"content"`
}

func (f *fileToolSet) readFile(_ context.Context, in readFileInput) (*readFileOutput, error) {
	target, err := resolveSafePath(f.workspaceRoot, in.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return &readFileOutput{Path: target, Exists: false, Content: ""}, nil
		}
		return nil, fmt.Errorf("stat path: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s (use list_dir to explore its contents)", target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return &readFileOutput{Path: target, Exists: true, Content: string(data)}, nil
}

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
type writeFileOutput struct {
	Path    string `json:"path"`
	Bytes   int    `json:"bytes"`
	Created bool   `json:"created"`
}

func (f *fileToolSet) enforceWritePolicy(target string) error {
	if len(f.policy.WriteAllowPrefixes) == 0 {
		return nil
	}
	rel, err := filepath.Rel(f.workspaceRoot, target)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	for _, p := range f.policy.WriteAllowPrefixes {
		pp := strings.TrimSuffix(filepath.ToSlash(p), "/")
		if rel == pp || strings.HasPrefix(rel, pp+"/") {
			return nil
		}
	}
	return fmt.Errorf("write target not allowed by policy: %s", rel)
}

func (f *fileToolSet) writeFile(_ context.Context, in writeFileInput) (*writeFileOutput, error) {
	target, err := resolveSafePath(f.workspaceRoot, in.Path)
	if err != nil {
		return nil, err
	}
	if err := f.enforceWritePolicy(target); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, fmt.Errorf("create parent directory: %w", err)
	}
	_, statErr := os.Stat(target)
	created := os.IsNotExist(statErr)
	if err := os.WriteFile(target, []byte(in.Content), 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return &writeFileOutput{Path: target, Bytes: len(in.Content), Created: created}, nil
}

type writeTempFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (f *fileToolSet) writeTempFile(ctx context.Context, in writeTempFileInput) (*writeFileOutput, error) {
	tempName := f.policy.TempDirName
	if tempName == "" {
		tempName = ".build-agent-tmp"
	}
	tempRoot := filepath.Join(f.workspaceRoot, tempName)
	tempTools := &fileToolSet{workspaceRoot: tempRoot}
	return tempTools.writeFile(ctx, writeFileInput{Path: in.Path, Content: in.Content})
}

func (f *fileToolSet) Tools() ([]tool.BaseTool, error) {
	listTool, err := toolutils.InferTool("list_dir", "列出目录内容", f.listDir)
	if err != nil {
		return nil, err
	}
	readTool, err := toolutils.InferTool("read_file", "读取文件内容", f.readFile)
	if err != nil {
		return nil, err
	}
	writeTool, err := toolutils.InferTool("write_file", "写入文件", f.writeFile)
	if err != nil {
		return nil, err
	}
	writeTempTool, err := toolutils.InferTool("write_temp_file", "写入临时文件", f.writeTempFile)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{listTool, readTool, writeTool, writeTempTool}, nil
}
