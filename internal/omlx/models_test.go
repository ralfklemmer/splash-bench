package omlx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

func TestModelsReadsSplashCompatibility(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[{"id":"Qwen3.8-27B-oQ4e-mtp","loaded":true,"model_path":"/m/Jundot/Q","estimated_size":16600000000,
			"settings":{"external_engine":"splash"},
			"external_engines":[{"id":"splash","label":"Splash","compatible":false,"reason":"needs draft","draft_repo":"incoai/Qwen3.8-27B-DFlash2"}]}]}`)
	})
	models, err := client.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := models[0]
	if m.ID != "Qwen3.8-27B-oQ4e-mtp" || !m.Loaded || *m.Settings.ExternalEngine != "splash" || m.ModelPath != "/m/Jundot/Q" {
		t.Errorf("model = %+v", m)
	}
	if len(m.ExternalEngines) != 1 || m.ExternalEngines[0].Compatible || m.ExternalEngines[0].Reason != "needs draft" ||
		m.ExternalEngines[0].DraftRepo != "incoai/Qwen3.8-27B-DFlash2" {
		t.Errorf("external engines = %+v", m.ExternalEngines)
	}
}

func TestHealthWorksWithoutKey(t *testing.T) {
	var gotAuth string
	var gotAuthSet bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotAuthSet = r.Header["Authorization"]
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"status":"healthy"}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "", server.Client())
	if err := client.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotAuthSet {
		t.Errorf("Authorization header sent without key: %q", gotAuth)
	}
}

func TestHealthReportsUnreachableServer(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", "", http.DefaultClient)
	if err := client.Health(context.Background()); err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestStreamTextDeliversContentAndReasoning(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
			`data: {"choices":[{"delta":{"reasoning_content":"hm"}}]}`,
			`data: {"choices":[{"delta":{"content":"pack"}}]}`,
			`data: {"choices":[{"delta":{"content":"age main"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":4}}`,
			`data: [DONE]`,
		}, "\n"))
	})
	var deltas []bench.Delta
	usage, err := client.StreamText(context.Background(), bench.ChatRequest{Model: "m"}, func(d bench.Delta) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatal(err)
	}
	want := []bench.Delta{{Reasoning: "hm"}, {Content: "pack"}, {Content: "age main"}}
	if len(deltas) != len(want) {
		t.Fatalf("deltas = %+v", deltas)
	}
	for i := range want {
		if deltas[i] != want[i] {
			t.Errorf("delta %d = %+v, want %+v", i, deltas[i], want[i])
		}
	}
	if usage.CompletionTokens != 4 {
		t.Errorf("usage = %+v", usage)
	}
}
