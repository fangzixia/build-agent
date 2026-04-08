package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

// ---- 共用 HTTP 客户端 ----

var webClient = &http.Client{
	Timeout: 20 * time.Second,
}

func doGet(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := webClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 || resp.StatusCode == 401 {
		return nil, fmt.Errorf("HTTP %d: 该网站拒绝访问，请跳过此链接换用其他来源", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512*1024))
}

// ---- HTML 工具函数（fetch_url 使用）----

var (
	reTag      = regexp.MustCompile(`<[^>]+>`)
	reEntities = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&nbsp;", " ",
		"&ndash;", "–", "&mdash;", "—",
	)
	reSpaces = regexp.MustCompile(`[ \t]+`)
	reLines  = regexp.MustCompile(`\n{3,}`)
)

func stripHTML(html string) string {
	html = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<(br|p|div|li|tr|h[1-6])[^>]*>`).ReplaceAllString(html, "\n")
	html = reTag.ReplaceAllString(html, "")
	html = reEntities.Replace(html)
	html = reSpaces.ReplaceAllString(html, " ")
	html = reLines.ReplaceAllString(strings.TrimSpace(html), "\n\n")
	return html
}

// ==================== web_search（Tavily API） ====================

const tavilyAPIKey = "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"
const tavilySearchURL = "https://api.tavily.com/search"

type webSearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type webSearchOutput struct {
	Query   string         `json:"query"`
	Results []searchResult `json:"results"`
}

// tavilyRequest 是发给 Tavily API 的请求体
type tavilyRequest struct {
	APIKey         string `json:"api_key"`
	Query          string `json:"query"`
	MaxResults     int    `json:"max_results"`
	SearchDepth    string `json:"search_depth"`        // "basic" 或 "advanced"
	IncludeAnswer  bool   `json:"include_answer"`      // 是否返回 AI 摘要
	IncludeContent bool   `json:"include_raw_content"` // 是否返回页面正文
}

// tavilyResponse 是 Tavily API 的响应体
type tavilyResponse struct {
	Query   string `json:"query"`
	Results []struct {
		Title      string  `json:"title"`
		URL        string  `json:"url"`
		Content    string  `json:"content"`     // 摘要/片段
		RawContent string  `json:"raw_content"` // 页面正文（include_raw_content=true 时有值）
		Score      float64 `json:"score"`
	} `json:"results"`
	Answer string `json:"answer"` // AI 生成的摘要（可选）
}

func tavilySearch(ctx context.Context, in webSearchInput) (*webSearchOutput, error) {
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	maxN := in.MaxResults
	if maxN <= 0 {
		maxN = 5
	}
	if maxN > 10 {
		maxN = 10
	}

	reqBody := tavilyRequest{
		APIKey:         tavilyAPIKey,
		Query:          q,
		MaxResults:     maxN,
		SearchDepth:    "basic",
		IncludeContent: true, // 返回页面正文，避免 fetch_url 拿不到 JS 渲染内容
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilySearchURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := webClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily API error %d: %s", resp.StatusCode, string(body))
	}

	var tavilyResp tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	out := &webSearchOutput{Query: q}
	for _, r := range tavilyResp.Results {
		// 优先用 raw_content（页面正文），截断到 3000 字符避免 token 过多
		// 没有 raw_content 时降级到 content（摘要片段）
		snippet := r.RawContent
		if snippet == "" {
			snippet = r.Content
		}
		if runes := []rune(snippet); len(runes) > 30000 {
			snippet = string(runes[:30000]) + "...[内容已截断，如需完整内容请用 fetch_url]"
		}
		out.Results = append(out.Results, searchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
		})
	}

	if len(out.Results) == 0 {
		return out, fmt.Errorf("no results found for %q", q)
	}
	return out, nil
}

type webSearchToolSet struct{}

func (w *webSearchToolSet) Tools() ([]tool.BaseTool, error) {
	t, err := toolutils.InferTool(
		"web_search",
		"通过 Tavily 搜索互联网，返回搜索结果列表（标题、URL、摘要）。适合搜索最新新闻、技术文档、实时信息等。",
		tavilySearch,
	)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{t}, nil
}

// ==================== fetch_url ====================

type fetchURLToolSet struct{}

type fetchURLInput struct {
	URL       string `json:"url"`
	MaxLength int    `json:"max_length,omitempty"`
}

type fetchURLOutput struct {
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
	Length  int    `json:"length"`
}

var reTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func (f *fetchURLToolSet) fetch(ctx context.Context, in fetchURLInput) (*fetchURLOutput, error) {
	rawURL := strings.TrimSpace(in.URL)
	if rawURL == "" {
		return nil, fmt.Errorf("url cannot be empty")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	maxLen := in.MaxLength
	if maxLen <= 0 {
		maxLen = 8000
	}
	if maxLen > 50000 {
		maxLen = 50000
	}

	body, err := doGet(ctx, rawURL, nil)
	if err != nil {
		// 将网络错误包装为正常输出，避免中断执行流
		return &fetchURLOutput{
			URL:     rawURL,
			Content: fmt.Sprintf("[fetch_url 失败] %v，请跳过此链接换用其他来源", err),
			Length:  0,
		}, nil
	}

	html := string(body)
	title := ""
	if m := reTitle.FindStringSubmatch(html); len(m) > 1 {
		title = strings.TrimSpace(stripHTML(m[1]))
	}

	text := stripHTML(html)
	if runes := []rune(text); len(runes) > maxLen {
		text = string(runes[:maxLen]) + "\n...[内容已截断]"
	}

	return &fetchURLOutput{
		URL:     rawURL,
		Title:   title,
		Content: text,
		Length:  len([]rune(text)),
	}, nil
}

func (f *fetchURLToolSet) Tools() ([]tool.BaseTool, error) {
	t, err := toolutils.InferTool(
		"fetch_url",
		"抓取指定 URL 的网页内容，返回去除 HTML 标签后的纯文本。可用于读取文档、新闻、API 文档等。",
		f.fetch,
	)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{t}, nil
}
