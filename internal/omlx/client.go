// Package omlx adapts the oMLX admin and OpenAI-compatible APIs to the
// ports of package bench.
package omlx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

// Doer sends HTTP requests; *http.Client satisfies it.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client talks to one oMLX server.
type Client struct {
	baseURL       string
	apiKey        string
	http          Doer
	pollInterval  time.Duration
	unloadTimeout time.Duration
}

func NewClient(baseURL, apiKey string, httpClient Doer) *Client {
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        apiKey,
		http:          httpClient,
		pollInterval:  time.Second,
		unloadTimeout: 5 * time.Minute,
	}
}

type modelStatus struct {
	ID        string `json:"id"`
	Loaded    bool   `json:"loaded"`
	IsLoading bool   `json:"is_loading"`
	Settings  struct {
		ExternalEngine *string `json:"external_engine"`
	} `json:"settings"`
}

func (c *Client) model(ctx context.Context, model string) (modelStatus, error) {
	var list struct {
		Models []modelStatus `json:"models"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/admin/api/models", nil, &list); err != nil {
		return modelStatus{}, err
	}
	for _, m := range list.Models {
		if m.ID == model {
			return m, nil
		}
	}
	return modelStatus{}, fmt.Errorf("Modell %q ist in oMLX nicht bekannt", model)
}

// CurrentEngine reads the engine oMLX has configured for model.
func (c *Client) CurrentEngine(ctx context.Context, model string) (bench.Engine, error) {
	m, err := c.model(ctx, model)
	if err != nil {
		return "", err
	}
	if m.Settings.ExternalEngine == nil {
		return bench.EngineOMLX, nil
	}
	return bench.Engine(*m.Settings.ExternalEngine), nil
}

// SetEngine configures model to run on engine; EngineOMLX is sent as null.
func (c *Client) SetEngine(ctx context.Context, model string, engine bench.Engine) error {
	body := map[string]any{"external_engine": nil}
	if engine != bench.EngineOMLX {
		body["external_engine"] = string(engine)
	}
	return c.doJSON(ctx, http.MethodPut, "/admin/api/models/"+url.PathEscape(model)+"/settings", body, nil)
}

// Unload frees model's memory and waits until oMLX has finished, because
// oMLX unloads asynchronously and rejects requests while an unload is
// pending. A model that is not loaded is not an error.
func (c *Client) Unload(ctx context.Context, model string) error {
	err := c.doJSON(ctx, http.MethodPost, "/admin/api/models/"+url.PathEscape(model)+"/unload", nil, nil)
	var status *StatusError
	if err != nil && !(errors.As(err, &status) && status.Code == http.StatusBadRequest && strings.Contains(status.Body, "not loaded")) {
		return err
	}
	return c.waitUntilUnloaded(ctx, model)
}

func (c *Client) waitUntilUnloaded(ctx context.Context, model string) error {
	ctx, cancel := context.WithTimeout(ctx, c.unloadTimeout)
	defer cancel()
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		m, err := c.model(ctx, model)
		if err != nil {
			return fmt.Errorf("wait for unload of %s: %w", model, err)
		}
		if !m.Loaded && !m.IsLoading {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for unload of %s: %w", model, ctx.Err())
		case <-ticker.C:
		}
	}
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Stream sends a streamed chat completion and reports every text chunk.
func (c *Client) Stream(ctx context.Context, req bench.ChatRequest, onChunk func()) (bench.Usage, error) {
	return c.StreamText(ctx, req, func(bench.Delta) { onChunk() })
}

// StreamText sends a streamed chat completion and hands every chunk that
// carries generated text to onDelta.
func (c *Client) StreamText(ctx context.Context, req bench.ChatRequest, onDelta func(bench.Delta)) (bench.Usage, error) {
	body := map[string]any{
		"model":                req.Model,
		"messages":             []map[string]string{{"role": "user", "content": req.Prompt}},
		"max_tokens":           req.MaxTokens,
		"stream":               true,
		"stream_options":       map[string]bool{"include_usage": true},
		"chat_template_kwargs": map[string]bool{"enable_thinking": req.Thinking},
	}
	resp, err := c.send(ctx, http.MethodPost, "/v1/chat/completions", body)
	if err != nil {
		return bench.Usage{}, err
	}
	defer resp.Body.Close()
	return readStream(resp.Body, onDelta)
}

func readStream(r io.Reader, onDelta func(bench.Delta)) (bench.Usage, error) {
	var usage bench.Usage
	gotUsage := false
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return usage, fmt.Errorf("decode stream chunk: %w", err)
		}
		if delta := chunk.delta(); !delta.Empty() {
			onDelta(delta)
		}
		if chunk.Usage != nil {
			usage = bench.Usage{PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens}
			gotUsage = true
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, fmt.Errorf("read stream: %w", err)
	}
	if !gotUsage {
		return usage, errors.New("Server hat keine Token-Zählung (usage) gesendet")
	}
	return usage, nil
}

func (c chatChunk) delta() bench.Delta {
	var d bench.Delta
	for _, choice := range c.Choices {
		d.Content += choice.Delta.Content
		d.Reasoning += choice.Delta.ReasoningContent
	}
	return d
}

// StatusError is a non-2xx answer from oMLX.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("oMLX antwortet mit HTTP %d: %s", e.Code, e.Message())
}

// Message is the error text from oMLX's JSON answer, with its line breaks,
// or the raw body when it is not such an answer.
func (e *StatusError) Message() string {
	var answer struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(e.Body), &answer) == nil && answer.Error.Message != "" {
		return answer.Error.Message
	}
	return e.Body
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	resp, err := c.send(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode %s %s: %w", method, path, err)
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return nil, fmt.Errorf("build %s %s: %w", method, path, err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		text, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s %s: %w", method, path, &StatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(text))})
	}
	return resp, nil
}
