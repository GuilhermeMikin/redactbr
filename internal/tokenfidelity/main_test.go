package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// These tests never leave the machine: real providers are replaced by local servers, and TestMain makes that a fact
// rather than an assumption, so that a broken check in the harness fails a test instead of reaching a provider.
func TestMain(m *testing.M) {
	local := http.DefaultTransport
	http.DefaultTransport = stubTransport(func(r *http.Request) (*http.Response, error) {
		if host := r.URL.Hostname(); host != "127.0.0.1" && host != "::1" && host != "localhost" {
			return nil, fmt.Errorf("test tried to reach %s", host)
		}
		return local.RoundTrip(r)
	})
	os.Exit(m.Run())
}

const testKey = "test-key-not-a-real-credential"

// The block must hold for the client the harness uses. The .invalid domain never resolves (RFC 2606), so this test
// reaches nothing even if the block is gone.
func TestTestsCannotLeaveTheMachine(t *testing.T) {
	resp, err := newHTTPClient(time.Second).Get("https://example.invalid/")
	if err == nil {
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "test tried to reach example.invalid") {
		t.Errorf("Get returned %v, want the error from the test's transport", err)
	}
}

var allValues = []string{cpfAna, cpfBruno, cpfCarla, cnpjDelta, cnpjBeta, mailAna, mailBruno}

func noEnv(t *testing.T) func(string) string {
	return func(name string) string {
		t.Errorf("read the environment variable %s", name)
		return ""
	}
}

func runHarness(t *testing.T, getenv func(string) string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, getenv, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestOfflineByDefault(t *testing.T) {
	code, out, errOut := runHarness(t, noEnv(t))
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if !strings.HasPrefix(out, "SIMULATED:") {
		t.Errorf("a simulated run does not say so first:\n%s", out)
	}
	for _, m := range fakeModels() {
		if !strings.Contains(out, m.name()) {
			t.Errorf("no line for %s", m.name())
		}
	}
}

func TestSimulatedBehaviorsReachTheirOutcomes(t *testing.T) {
	want := map[string]outcome{
		"perfect": outcomeExact, "uppercase": outcomeCase, "html_escaped": outcomeEscaped, "spaced": outcomeSpacing,
		"new_separator": outcomeSeparator, "square_brackets": outcomeBrackets, "typo_in_id": outcomeIDEdited,
		"drops_tokens": outcomeMissing, "invents_tokens": outcomeExact, "repeats_and_mangles": outcomeMixed,
	}
	for _, m := range fakeModels() {
		behavior := strings.TrimPrefix(m.name(), "simulated/")
		for _, sc := range scenarios {
			res, err := runCase(context.Background(), m, sc, variants[0], 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range res.Tokens {
				if s.Expected && s.Outcome != want[behavior] {
					t.Errorf("%s, %s: outcome %s, want %s", behavior, sc.name, s.Outcome, want[behavior])
				}
			}
			if restorable := want[behavior] == outcomeExact || want[behavior] == outcomeMixed; res.Restored != restorable {
				t.Errorf("%s, %s: all values restored = %v", behavior, sc.name, res.Restored)
			}
			if !res.StreamAgrees {
				t.Errorf("%s, %s: RestoreWriter disagreed with Restore", behavior, sc.name)
			}
			if res.RawValue {
				t.Errorf("%s, %s: a value appeared in the answer", behavior, sc.name)
			}
			if inventing := behavior == "invents_tokens"; inventing != (len(res.Invented) > 0) {
				t.Errorf("%s, %s: invented tokens %v", behavior, sc.name, res.Invented)
			}
		}
	}
}

// captureModel records what would be sent.
type captureModel struct{ sent *[]request }

func (captureModel) name() string { return "capture" }

func (m captureModel) complete(_ context.Context, req request) (response, error) {
	*m.sent = append(*m.sent, req)
	return response{}, nil
}

func TestValuesNeverReachAModel(t *testing.T) {
	var sent []request
	for _, sc := range scenarios {
		for _, v := range variants {
			if _, err := runCase(context.Background(), captureModel{&sent}, sc, v, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(sent) != len(scenarios)*len(variants) {
		t.Fatalf("%d requests, want %d", len(sent), len(scenarios)*len(variants))
	}
	digits := regexp.MustCompile(`[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2}`)
	for _, req := range sent {
		texts := []string{req.system}
		for _, tr := range req.turns {
			texts = append(texts, tr.text)
		}
		for _, text := range texts {
			if digits.MatchString(text) || strings.Contains(text, "@example.") {
				t.Errorf("a value is in the text to send: %q", text)
			}
			for _, value := range allValues {
				if strings.Contains(text, value) {
					t.Errorf("a value is in the text to send: %q", text)
				}
			}
		}
	}
}

func TestScenariosAreWellFormed(t *testing.T) {
	names := make(map[string]bool)
	for _, sc := range scenarios {
		if names[sc.name] {
			t.Errorf("repeated scenario name %s", sc.name)
		}
		names[sc.name] = true
		var all strings.Builder
		for _, tr := range sc.turns {
			all.WriteString(tr.text)
		}
		for _, value := range sc.values {
			if !strings.Contains(all.String(), value) {
				t.Errorf("%s expects a value that is not in its text: %s", sc.name, value)
			}
		}
		if sc.turns[len(sc.turns)-1].role != "user" {
			t.Errorf("%s does not end with a user turn", sc.name)
		}
	}
}

func TestPlanSendsNothingAndReadsNoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the plan sent a request")
	}))
	defer srv.Close()
	for _, provider := range []string{"anthropic", "openai"} {
		code, out, errOut := runHarness(t, noEnv(t), "-provider", provider, "-model", "some-model", "-base-url", srv.URL, "-repeats", "2")
		if code != 0 || errOut != "" {
			t.Fatalf("%s: exit %d, stderr %q", provider, code, errOut)
		}
		for _, want := range []string{"Nothing was sent", "requests: 48", "12 scenarios", "196608 in total", keyVariable(provider)} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: the plan does not mention %q:\n%s", provider, want, out)
			}
		}
	}
}

func TestRefusesInvalidRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("an invalid run sent a request")
	}))
	defer srv.Close()
	empty := func(string) string { return "" }
	key := func(string) string { return testKey }
	tests := []struct {
		name   string
		getenv func(string) string
		args   []string
		code   int
		want   string
	}{
		{"no key", empty, []string{"-provider", "anthropic", "-model", "m", "-live"}, 1, "ANTHROPIC_API_KEY is not set"},
		{"no key for OpenAI", empty, []string{"-provider", "openai", "-model", "m", "-live"}, 1, "OPENAI_API_KEY is not set"},
		{"no key for Anthropic at another URL", empty, []string{"-provider", "anthropic", "-model", "m", "-live", "-base-url", srv.URL}, 1, "ANTHROPIC_API_KEY is not set"},
		{"no model", key, []string{"-provider", "anthropic", "-live"}, 2, "-model is required"},
		{"unknown provider", key, []string{"-provider", "acme", "-model", "m"}, 2, "unknown provider"},
		{"live without provider", key, []string{"-live"}, 2, "need -provider"},
		{"over the request cap", key, []string{"-provider", "anthropic", "-model", "m", "-live", "-base-url", srv.URL, "-repeats", "5"}, 2, "more than -max-requests"},
		{"over the request cap by far", key, []string{"-provider", "anthropic", "-model", "m", "-repeats", "768614336404564651"}, 2, "more than -max-requests"},
		{"plain HTTP", key, []string{"-provider", "openai", "-model", "m", "-base-url", "http://example.com/v1"}, 2, "use https"},
		{"key over plain HTTP", key, []string{"-provider", "openai", "-model", "m", "-live", "-base-url", "http://example.com/v1"}, 2, "use https"},
		{"credentials in the URL", key, []string{"-provider", "openai", "-model", "m", "-base-url", "https://user:s3cret@api.example.com/v1"}, 2, "must not contain credentials"},
		{"stray argument", key, []string{"extra"}, 2, "unexpected argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runHarness(t, tt.getenv, tt.args...)
			if code != tt.code || !strings.Contains(errOut, tt.want) {
				t.Errorf("exit %d, stderr %q; want exit %d and %q", code, errOut, tt.code, tt.want)
			}
			if strings.Contains(out+errOut, "s3cret") {
				t.Errorf("credentials in a URL were printed: %s%s", out, errOut)
			}
		})
	}
}

func TestCheckEndpoint(t *testing.T) {
	for base, ok := range map[string]bool{
		"https://api.example.com/v1": true, "http://localhost:11434/v1": true, "http://127.0.0.1:8080": true, "http://[::1]:8080": true,
		"http://api.example.com": false, "ftp://localhost": false, "not a url": false, "": false, "https://user:secret@api.example.com": false,
	} {
		if err := checkEndpoint(base); (err == nil) != ok {
			t.Errorf("checkEndpoint(%q) = %v, want ok = %v", base, err, ok)
		}
	}
}

// lastUserTokens returns the tokens in the user turns of an API request body, which the fake servers answer
// back.
func lastUserTokens(t *testing.T, body map[string]any) []string {
	t.Helper()
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("no messages in the request: %v", body)
	}
	var out []string
	for _, m := range messages {
		msg := m.(map[string]any)
		if msg["role"] == "user" {
			out = append(out, tokenPattern.FindAllString(msg["content"].(string), -1)...)
		}
	}
	return out
}

// checkNoValues fails if a request body has a value or a forbidden sampling parameter in it.
func checkNoValues(t *testing.T, raw []byte) {
	t.Helper()
	for _, value := range allValues {
		if bytes.Contains(raw, []byte(value)) {
			t.Errorf("a value was sent: %s", raw)
		}
	}
	for _, field := range []string{`"temperature"`, `"top_p"`, `"top_k"`} {
		if bytes.Contains(raw, []byte(field)) {
			t.Errorf("request has %s: %s", field, raw)
		}
	}
}

func readOut(t *testing.T, path string) (first map[string]any, results []result) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		if first == nil {
			if err := json.Unmarshal(sc.Bytes(), &first); err != nil {
				t.Fatal(err)
			}
			continue
		}
		var r result
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		results = append(results, r)
	}
	return first, results
}

func TestAnthropicRequestsAndResponses(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		raw, _ := io.ReadAll(r.Body)
		checkNoValues(t, raw)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		for name, want := range map[string]string{
			"x-api-key": testKey, "anthropic-version": "2026-10-21", "Content-Type": "application/json", "anthropic-workspace-id": "wrkspc_test",
		} {
			if got := r.Header.Get(name); got != want {
				t.Errorf("header %s = %q, want %q", name, got, want)
			}
		}
		if body["model"] != "some-model" || body["max_tokens"] != float64(123) {
			t.Errorf("unexpected body: %s", raw)
		}
		reply := strings.Join(lastUserTokens(t, body), " and ")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "thinking", "thinking": "not part of the answer <CPF_k7q2xm9pb3>"},
				{"type": "text", "text": "Sure: " + reply},
				{"type": "text", "text": "."},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "out.jsonl")
	env := func(name string) string {
		switch name {
		case "ANTHROPIC_API_KEY":
			return testKey
		case "ANTHROPIC_WORKSPACE_ID":
			return "wrkspc_test"
		}
		return ""
	}
	code, out, errOut := runHarness(t, env, "-provider", "anthropic", "-model", "some-model", "-base-url", srv.URL,
		"-live", "-repeats", "1", "-max-tokens", "123", "-out", path)
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if requests != len(scenarios)*len(variants) {
		t.Errorf("%d requests, want %d", requests, len(scenarios)*len(variants))
	}
	first, results := readOut(t, path)
	if harness, ok := first["harness"].(map[string]any); !ok || harness["model"] != "some-model" || harness["anthropic_version"] != "2026-10-21" {
		t.Errorf("unexpected metadata: %v", first)
	}
	if len(results) != requests {
		t.Fatalf("%d results, want %d", len(results), requests)
	}
	for _, r := range results {
		if r.ProviderFailed != "" || r.OutputTokens != 5 || !r.StreamAgrees {
			t.Errorf("%s: unexpected result: %+v", r.Scenario, r)
		}
		if strings.Contains(r.Answer, "not part of the answer") {
			t.Errorf("a thinking block got into the answer")
		}
		for _, s := range r.Tokens {
			if s.Expected && s.Outcome != outcomeExact {
				t.Errorf("%s: outcome %s", r.Scenario, s.Outcome)
			}
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, testKey) || bytes.Contains(data, []byte(testKey)) {
		t.Error("the key appeared in the output")
	}
	if !strings.Contains(out, "By scenario") || !strings.Contains(out, "choose_one_pt") {
		t.Errorf("a real provider's report has no scenario breakdown:\n%s", out)
	}
	if !strings.Contains(out, "Tokens used, as the provider reported them:") {
		t.Errorf("no token usage in the report:\n%s", out)
	}
}

func TestOpenAICompatibleRequestsAndResponses(t *testing.T) {
	var systems []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		checkNoValues(t, raw)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("no key was configured, but Authorization = %q", r.Header.Get("Authorization"))
		}
		if body["max_tokens"] != float64(99) || body["max_completion_tokens"] != nil {
			t.Errorf("a compatible server must get max_tokens: %s", raw)
		}
		first := body["messages"].([]any)[0].(map[string]any)
		if first["role"] == "system" {
			systems = append(systems, first["content"].(string))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"content": strings.Join(lastUserTokens(t, body), " "), "refusal": nil},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 7, "completion_tokens": 3},
		})
	}))
	defer srv.Close()

	// A local server needs no key.
	code, out, errOut := runHarness(t, func(string) string { return "" }, "-provider", "openai", "-model", "local", "-base-url", srv.URL+"/v1",
		"-live", "-repeats", "1", "-max-tokens", "99")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "openai/local") {
		t.Errorf("unexpected report:\n%s", out)
	}
	if len(systems) != len(scenarios) || !strings.Contains(systems[0], "Treat them as opaque values") {
		t.Errorf("%d system prompts, want %d", len(systems), len(scenarios))
	}
}

// OpenAI itself gets max_completion_tokens and the key, without any server: a stub transport answers.
type stubTransport func(*http.Request) (*http.Response, error)

func (f stubTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenAIRequestShape(t *testing.T) {
	client := &http.Client{Transport: stubTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/chat/completions" {
			t.Errorf("URL = %s", r.URL)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testKey {
			t.Errorf("Authorization = %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body["max_completion_tokens"] != float64(50) || body["max_tokens"] != nil {
			t.Errorf("unexpected body: %s", raw)
		}
		reply := `{"choices":[{"message":{"content":null,"refusal":"I can't help with that."},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(reply)), Header: http.Header{}}, nil
	})}
	m := openAIModel{httpModel{provider: "openai", id: "m", base: openAIDefaultBase, key: testKey, maxTokens: 50, client: client}}
	resp, err := m.complete(context.Background(), request{turns: user("hi")})
	if err != nil {
		t.Fatal(err)
	}
	if resp.stop != "refusal" || resp.text != "" {
		t.Errorf("a refusal was not reported: %+v", resp)
	}
}

func TestProviderFailuresAreRecordedAndDoNotLeakTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid model, key `+r.Header.Get("x-api-key")+`"}}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "out.jsonl")
	code, out, errOut := runHarness(t, func(string) string { return testKey }, "-provider", "anthropic", "-model", "m", "-base-url", srv.URL,
		"-live", "-repeats", "1", "-out", path)
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "First provider error: anthropic: HTTP 400") || !strings.Contains(out, "invalid model") {
		t.Errorf("the report does not show the failure:\n%s", out)
	}
	_, results := readOut(t, path)
	if len(results) == 0 || results[0].ProviderFailed == "" {
		t.Errorf("failure not recorded: %+v", results)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(out+errOut+string(data), testKey) {
		t.Error("the key appeared in the output")
	}
}

func TestRetriesRateLimitsAndServerErrors(t *testing.T) {
	statuses := []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusOK}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		status := statuses[calls]
		calls++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{}}`))
	}))
	defer srv.Close()
	m := anthropicModel{httpModel{provider: "anthropic", id: "m", base: srv.URL, key: testKey, maxTokens: 1, client: srv.Client()}}
	resp, err := m.complete(context.Background(), request{turns: user("hi")})
	if err != nil || resp.text != "ok" || calls != 3 {
		t.Errorf("complete = (%+v, %v) after %d calls", resp, err, calls)
	}

	calls = 0
	statuses = []int{http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusOK}
	if _, err := m.complete(context.Background(), request{turns: user("hi")}); err == nil || calls != 3 {
		t.Errorf("gave up after %d calls with error %v, want 3 calls and an error", calls, err)
	}
}

func TestDoesNotFollowRedirects(t *testing.T) {
	var leaked []string
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		leaked = append(leaked, r.Header.Get("x-api-key"))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	m := anthropicModel{httpModel{provider: "anthropic", id: "m", base: srv.URL, key: testKey, maxTokens: 1, client: newHTTPClient(0)}}
	if _, err := m.complete(context.Background(), request{turns: user("hi")}); err == nil || !strings.Contains(err.Error(), "307") {
		t.Errorf("complete returned %v, want an error with the status", err)
	}
	if len(leaked) > 0 {
		t.Error("a redirect was followed with the key")
	}
}

func TestKeyEchoedInAnAnswerIsRemoved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "your key is " + r.Header.Get("x-api-key")}},
		})
	}))
	defer srv.Close()
	m := anthropicModel{httpModel{provider: "anthropic", id: "m", base: srv.URL, key: testKey, maxTokens: 1, client: srv.Client()}}
	resp, err := m.complete(context.Background(), request{turns: user("hi")})
	if err != nil || strings.Contains(resp.text, testKey) || !strings.Contains(resp.text, "[key]") {
		t.Errorf("complete = (%+v, %v)", resp, err)
	}
}

func TestRetryWaitIsCapped(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "100000")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	m := anthropicModel{httpModel{provider: "anthropic", id: "m", base: srv.URL, key: testKey, maxTokens: 1, client: srv.Client()}}
	if _, err := m.complete(ctx, request{turns: user("hi")}); !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Errorf("complete returned %v after %d calls, want the context's error while waiting", err, calls)
	}
	if maxRetryWait > 5*time.Minute {
		t.Errorf("maxRetryWait = %v", maxRetryWait)
	}
}

func TestOpenAITrailingSlashIsStillOpenAI(t *testing.T) {
	client := &http.Client{Transport: stubTransport(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if !bytes.Contains(raw, []byte(`"max_completion_tokens"`)) {
			t.Errorf("unexpected body: %s", raw)
		}
		reply := `{"choices":[{"message":{"content":"partial"},"finish_reason":"content_filter"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(reply)), Header: http.Header{}}, nil
	})}
	m := openAIModel{httpModel{provider: "openai", id: "m", base: openAIDefaultBase + "/", key: testKey, maxTokens: 5, client: client}}
	resp, err := m.complete(context.Background(), request{turns: user("hi")})
	if err != nil || resp.stop != "refusal" {
		t.Errorf("a content filter was not reported as a refusal: (%+v, %v)", resp, err)
	}
}

func TestTallyCounters(t *testing.T) {
	results := []result{
		{Stop: "refusal", StreamAgrees: true},
		{Stop: "max_tokens", StreamAgrees: true},
		{Stop: "length", StreamAgrees: true},
		{RawValue: true, Restored: true, StreamAgrees: false, Invented: []string{"<CPF_0000000000>"}},
		{ProviderFailed: "boom"},
	}
	var tl tally
	for _, r := range results {
		tl.add(r)
	}
	if tl.requests != 5 || tl.refused != 1 || tl.truncated != 2 || tl.rawValues != 1 || tl.streamDisagrees != 1 || tl.invented != 1 || tl.failed != 1 || tl.complete != 1 {
		t.Errorf("unexpected tally: %+v", tl)
	}
}

// A value that reached a model should never come back, but if it does the report must say so.
func TestRawValueInAnAnswerIsReported(t *testing.T) {
	echo := fakeModel{"answers_with_a_value", func(string) string { return cpfAna }}
	res, err := runCase(context.Background(), echo, scenarios[0], variants[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.RawValue {
		t.Error("a value in an answer was not reported")
	}
}

func TestReportCountsWrongTokens(t *testing.T) {
	sc := scenarios[slices.IndexFunc(scenarios, func(s scenario) bool { return s.name == "choose_one_pt" })]
	wrong := fakeModel{"answers_with_every_token", func(tok string) string { return tok }}
	res, err := runCase(context.Background(), wrong, sc, variants[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	var tl tally
	tl.add(res)
	if tl.wrongToken != 1 || tl.tokens != 1 {
		t.Errorf("wrong tokens = %d, expected tokens = %d; want 1 and 1", tl.wrongToken, tl.tokens)
	}
}
