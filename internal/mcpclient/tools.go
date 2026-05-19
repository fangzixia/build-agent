package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"build-agent/internal/config"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AppendTools 为每个已启用的 MCP 服务器建立会话、列出 tools，并返回可关闭所有会话的 cleanup。
// 单个服务器连接或 tools/list 失败时仅跳过该服务器并记录日志，其余服务器仍可用（避免拖垮整个智能体）。
// workspaceRoot 用于解析相对 Cwd；defaultCallTimeoutSec 在配置未指定 CallTimeoutSec 时使用。
func AppendTools(ctx context.Context, servers map[string]config.MCPServerConfig, workspaceRoot string, defaultCallTimeoutSec int) ([]tool.BaseTool, func(), error) {
	if len(servers) == 0 {
		return nil, func() {}, nil
	}

	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	var closers []func()
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	usedNames := make(map[string]struct{})
	var out []tool.BaseTool

	for _, serverKey := range names {
		sc := servers[serverKey]
		if sc.Disabled || strings.TrimSpace(sc.Command) == "" {
			continue
		}

		sess, closer, err := connectServer(ctx, serverKey, sc, workspaceRoot)
		if err != nil {
			log.Printf("mcp: skip server %q (connect): %v", serverKey, err)
			continue
		}

		listTO := callTimeoutDuration(sc.CallTimeoutSec, defaultCallTimeoutSec)
		listCtx := ctx
		var listCancel context.CancelFunc
		if listTO > 0 {
			listCtx, listCancel = context.WithTimeout(ctx, listTO)
		}
		toolsRes, err := sess.ListTools(listCtx, nil)
		if listCancel != nil {
			listCancel()
		}
		if err != nil {
			closer()
			log.Printf("mcp: skip server %q (tools/list): %v", serverKey, err)
			continue
		}

		closers = append(closers, closer)

		callTO := callTimeoutDuration(sc.CallTimeoutSec, defaultCallTimeoutSec)
		for _, tl := range toolsRes.Tools {
			if tl == nil || tl.Name == "" {
				continue
			}
			params, err := inputSchemaToParams(tl.InputSchema)
			if err != nil {
				log.Printf("mcp: skip server %q tool %q (schema): %v", serverKey, tl.Name, err)
				continue
			}
			display := uniqueToolName(usedNames, serverKey, tl.Name)
			desc := strings.TrimSpace(tl.Description)
			if desc == "" {
				desc = fmt.Sprintf("MCP tool %q (server %q)", tl.Name, serverKey)
			} else {
				desc = fmt.Sprintf("[MCP:%s] %s", serverKey, desc)
			}
			out = append(out, &mcpTool{
				session:     sess,
				origName:    tl.Name,
				displayName: display,
				desc:        desc,
				params:      params,
				callTimeout: callTO,
			})
		}
	}

	return out, closeAll, nil
}

func callTimeoutDuration(sec int, defaultSec int) time.Duration {
	if sec > 0 {
		return time.Duration(sec) * time.Second
	}
	if defaultSec > 0 {
		return time.Duration(defaultSec) * time.Second
	}
	return 90 * time.Second
}

func connectServer(ctx context.Context, _ /* serverKey */ string, sc config.MCPServerConfig, workspaceRoot string) (*mcp.ClientSession, func(), error) {
	startSec := sc.StartTimeoutSec
	if startSec <= 0 {
		startSec = 30
	}
	connCtx, cancel := context.WithTimeout(ctx, time.Duration(startSec)*time.Second)
	defer cancel()

	cwd := strings.TrimSpace(sc.Cwd)
	if cwd == "" {
		cwd = workspaceRoot
	} else if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(workspaceRoot, cwd)
	}

	cmd := exec.CommandContext(connCtx, sc.Command, sc.Args...)
	cmd.Dir = cwd
	cmd.Env = mergeEnv(os.Environ(), sc.Env)

	transport := &mcp.CommandTransport{Command: cmd}
	client := mcp.NewClient(&mcp.Implementation{Name: "build-agent", Version: "1.0.0"}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{},
	})

	sess, err := client.Connect(connCtx, transport, nil)
	if err != nil {
		return nil, nil, err
	}
	closer := func() { _ = sess.Close() }
	return sess, closer, nil
}

func mergeEnv(base []string, override map[string]string) []string {
	if len(override) == 0 {
		return base
	}
	m := make(map[string]string)
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i > 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range override {
		m[k] = v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}

func inputSchemaToParams(inputSchema any) (*schema.ParamsOneOf, error) {
	fallback := schema.NewParamsOneOfByJSONSchema(&jsonschema.Schema{Type: string(schema.Object)})
	if inputSchema == nil {
		return fallback, nil
	}
	raw, err := json.Marshal(inputSchema)
	if err != nil {
		return fallback, nil
	}
	var js jsonschema.Schema
	if err := json.Unmarshal(raw, &js); err != nil {
		return fallback, nil
	}
	if js.Type == "" && len(js.TypeEnhanced) == 0 {
		js.Type = string(schema.Object)
	}
	return schema.NewParamsOneOfByJSONSchema(&js), nil
}

func uniqueToolName(used map[string]struct{}, serverKey, mcpName string) string {
	base := "mcp_" + sanitizePart(serverKey) + "__" + sanitizePart(mcpName)
	name := base
	for i := 2; ; i++ {
		if _, ok := used[name]; !ok {
			used[name] = struct{}{}
			return name
		}
		name = fmt.Sprintf("%s_%d", base, i)
	}
}

func sanitizePart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "x"
	}
	return out
}

type mcpTool struct {
	session     *mcp.ClientSession
	origName    string
	displayName string
	desc        string
	params      *schema.ParamsOneOf
	callTimeout time.Duration
}

func (t *mcpTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        t.displayName,
		Desc:        t.desc,
		ParamsOneOf: t.params,
	}, nil
}

func (t *mcpTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args any
	s := strings.TrimSpace(argumentsInJSON)
	if s == "" || s == "{}" {
		args = map[string]any{}
	} else {
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(s), &raw); err != nil {
			return "", fmt.Errorf("invalid tool arguments JSON: %w", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return "", fmt.Errorf("tool arguments must be a JSON object: %w", err)
		}
		args = m
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if t.callTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, t.callTimeout)
		defer cancel()
	}

	res, err := t.session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      t.origName,
		Arguments: args,
	})
	if err != nil {
		return "", err
	}
	return formatCallToolResult(res)
}

func formatCallToolResult(r *mcp.CallToolResult) (string, error) {
	if r == nil {
		return "", nil
	}
	var parts []string
	if r.IsError {
		parts = append(parts, "[error]")
	}
	if r.StructuredContent != nil {
		b, err := json.MarshalIndent(r.StructuredContent, "", "  ")
		if err != nil {
			parts = append(parts, fmt.Sprintf("%v", r.StructuredContent))
		} else {
			parts = append(parts, string(b))
		}
	}
	for _, c := range r.Content {
		parts = append(parts, formatContent(c))
	}
	if len(parts) == 0 {
		return "{}", nil
	}
	return strings.Join(parts, "\n"), nil
}

func formatContent(c mcp.Content) string {
	switch x := c.(type) {
	case *mcp.TextContent:
		return x.Text
	case *mcp.ImageContent:
		return fmt.Sprintf("[image mime=%s bytes=%d]", x.MIMEType, len(x.Data))
	case *mcp.AudioContent:
		return fmt.Sprintf("[audio mime=%s bytes=%d]", x.MIMEType, len(x.Data))
	default:
		b, err := json.Marshal(c)
		if err != nil {
			return fmt.Sprintf("%v", c)
		}
		return string(b)
	}
}
