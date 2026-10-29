package omlx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(server.URL+"/", "secret", server.Client())
}

func TestCurrentEngine(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `{"models":[
			{"id":"a","settings":{"external_engine":null}},
			{"id":"b","settings":{"external_engine":"splash"}}]}`)
	})
	tests := []struct {
		model   string
		want    bench.Engine
		wantErr bool
	}{
		{"a", bench.EngineOMLX, false},
		{"b", bench.EngineSplash, false},
		{"missing", "", true},
	}
	for _, tt := range tests {
		got, err := client.CurrentEngine(context.Background(), tt.model)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("CurrentEngine(%q) = %q, %v", tt.model, got, err)
		}
	}
}

func TestSetEngineSendsNullForOMLX(t *testing.T) {
	tests := []struct {
		engine bench.Engine
		want   string
	}{
		{bench.EngineOMLX, `{"external_engine":null}`},
		{bench.EngineSplash, `{"external_engine":"splash"}`},
	}
	for _, tt := range tests {
		var gotBody, gotPath, gotMethod string
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody, gotPath, gotMethod = string(body), r.URL.Path, r.Method
			io.WriteString(w, `{}`)
		})
		if err := client.SetEngine(context.Background(), "Qwen3.8-27B-oQ4e-mtp", tt.engine); err != nil {
			t.Fatal(err)
		}
		if gotMethod != http.MethodPut || gotPath != "/admin/api/models/Qwen3.8-27B-oQ4e-mtp/settings" || gotBody != tt.want {
			t.Errorf("%s %s %s, want body %s", gotMethod, gotPath, gotBody, tt.want)
		}
	}
}

func TestSetEnginePassesServerReason(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"Splash needs the draft"}`, http.StatusBadRequest)
	})
	err := client.SetEngine(context.Background(), "m", bench.EngineSplash)
	if err == nil || !strings.Contains(err.Error(), "Splash needs the draft") {
		t.Errorf("err = %v, want server reason", err)
	}
}

func TestUnloadIgnoresModelNotLoaded(t *testing.T) {
	tests := []struct {
		status  int
		body    string
		wantErr bool
	}{
		{http.StatusOK, `{}`, false},
		{http.StatusBadRequest, `{"detail":"Model not loaded: m"}`, false},
		{http.StatusConflict, `{"detail":"Model still loading: m"}`, true},
	}
	for _, tt := range tests {
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				io.WriteString(w, `{"models":[{"id":"m","loaded":false,"is_loading":false}]}`)
				return
			}
			w.WriteHeader(tt.status)
			io.WriteString(w, tt.body)
		})
		if err := client.Unload(context.Background(), "m"); (err != nil) != tt.wantErr {
			t.Errorf("status %d: err = %v", tt.status, err)
		}
	}
}

func TestUnloadWaitsUntilModelIsUnloaded(t *testing.T) {
	polls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			io.WriteString(w, `{}`)
			return
		}
		polls++
		loaded := polls < 3 // unload still pending on the first two polls
		fmt.Fprintf(w, `{"models":[{"id":"m","loaded":%t,"is_loading":false}]}`, loaded)
	})
	client.pollInterval = time.Millisecond

	if err := client.Unload(context.Background(), "m"); err != nil {
		t.Fatal(err)
	}
	if polls != 3 {
		t.Errorf("polls = %d, want 3", polls)
	}
}

func TestUnloadTimesOutWhileModelStaysLoaded(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			io.WriteString(w, `{}`)
			return
		}
		io.WriteString(w, `{"models":[{"id":"m","loaded":true}]}`)
	})
	client.pollInterval = time.Millisecond
	client.unloadTimeout = 20 * time.Millisecond

	err := client.Unload(context.Background(), "m")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
}

func TestStreamCountsTextChunksAndReadsUsage(t *testing.T) {
	var request map[string]any
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&request)
		io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
			``,
			`data: {"choices":[{"delta":{"reasoning_content":"hm"}}]}`,
			`data: {"choices":[{"delta":{"content":"Hallo"}}]}`,
			`: keep-alive`,
			`data: {"choices":[{"delta":{"content":" Welt"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3}}`,
			`data: [DONE]`,
		}, "\n"))
	})

	chunks := 0
	usage, err := client.Stream(context.Background(), bench.ChatRequest{Model: "m", Prompt: "hi", MaxTokens: 64}, func() { chunks++ })
	if err != nil {
		t.Fatal(err)
	}
	if chunks != 3 {
		t.Errorf("chunks = %d, want 3", chunks)
	}
	if usage != (bench.Usage{PromptTokens: 12, CompletionTokens: 3}) {
		t.Errorf("usage = %+v", usage)
	}
	if request["stream"] != true || request["max_tokens"] != float64(64) {
		t.Errorf("request = %v", request)
	}
	kwargs, _ := request["chat_template_kwargs"].(map[string]any)
	if kwargs["enable_thinking"] != false {
		t.Errorf("enable_thinking = %v, want false", kwargs["enable_thinking"])
	}
}

func TestStreamFailsWithoutUsage(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\ndata: [DONE]\n")
	})
	if _, err := client.Stream(context.Background(), bench.ChatRequest{Model: "m"}, func() {}); err == nil {
		t.Error("expected error for missing usage")
	}
}
