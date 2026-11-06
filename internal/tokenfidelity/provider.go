package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A request is one model call; a response, what came back. Sampling parameters are left out on purpose: the
// point is what a pipeline gets by default, and API version 2026-10-21 of the Claude API rejects them.
type request struct {
	system string
	turns  []turn
}

type response struct {
	text         string
	stop         string // the provider's stop reason, "refusal" for a refusal on any provider
	inputTokens  int
	outputTokens int
}

type model interface {
	name() string
	complete(ctx context.Context, req request) (response, error)
}

const (
	anthropicDefaultBase = "https://api.anthropic.com"
	anthropicVersion     = "2026-10-21"
	openAIDefaultBase    = "https://api.openai.com/v1"
)

// httpModel holds what the two HTTP providers share. The key is sent only in the request headers and never
// printed; redirects are not followed, because net/http would carry a custom header such as x-api-key to
// the new host.
type httpModel struct {
	provider, id, base, key string
	maxTokens               int
	workspace               string // Anthropic only: anthropic-workspace-id, for keys not scoped to a workspace
	client                  *http.Client
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (m httpModel) name() string { return m.provider + "/" + m.id }

// checkEndpoint refuses to send a key in the clear: plain HTTP is for a local server only.
func checkEndpoint(base, key string) error {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid base URL %q", base)
	}
	host := u.Hostname()
	local := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return fmt.Errorf("base URL %q: use https, or http on localhost", base)
	}
	if key != "" && u.User != nil {
		return errors.New("base URL must not contain credentials")
	}
	return nil
}

// post sends a JSON request, retrying rate limits and server errors a few times, and decodes the response.
func (m httpModel) post(ctx context.Context, url string, headers map[string]string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	const attempts = 3
	for attempt := 1; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := m.client.Do(req)
		if err != nil {
			return fmt.Errorf("%s: %w", m.provider, err)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("%s: reading the response: %w", m.provider, err)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("%s: decoding the response: %w", m.provider, err)
			}
			return nil
		case attempt < attempts && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500):
			wait := time.Duration(attempt) * 2 * time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
				wait = time.Duration(s) * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		default:
			return fmt.Errorf("%s: HTTP %d: %s", m.provider, resp.StatusCode, truncate(string(data), 300))
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// anthropicModel calls the Claude Messages API with net/http, because the module has no dependencies
// (ADR-0002). See docs/token-fidelity.md for the sources of the request shape. No fallback model is
// configured: a refused request is reported, not answered by a model other than the one being measured.
type anthropicModel struct{ httpModel }

func (m anthropicModel) complete(ctx context.Context, req request) (response, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	in := struct {
		Model     string    `json:"model"`
		MaxTokens int       `json:"max_tokens"`
		System    string    `json:"system,omitempty"`
		Messages  []message `json:"messages"`
	}{Model: m.id, MaxTokens: m.maxTokens, System: req.system}
	for _, t := range req.turns {
		in.Messages = append(in.Messages, message{t.role, t.text})
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	headers := map[string]string{"x-api-key": m.key, "anthropic-version": anthropicVersion}
	if m.workspace != "" {
		headers["anthropic-workspace-id"] = m.workspace
	}
	if err := m.post(ctx, strings.TrimRight(m.base, "/")+"/v1/messages", headers, in, &out); err != nil {
		return response{}, err
	}
	var text strings.Builder
	for _, block := range out.Content {
		if block.Type == "text" { // thinking blocks are not part of the answer
			text.WriteString(block.Text)
		}
	}
	return response{text.String(), out.StopReason, out.Usage.InputTokens, out.Usage.OutputTokens}, nil
}

// openAIModel calls the Chat Completions API of OpenAI or of a compatible server (-base-url), such as a local
// one. OpenAI deprecated max_tokens in favor of max_completion_tokens; compatible servers document max_tokens.
type openAIModel struct{ httpModel }

func (m openAIModel) complete(ctx context.Context, req request) (response, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	in := map[string]any{"model": m.id}
	if m.base == openAIDefaultBase {
		in["max_completion_tokens"] = m.maxTokens
	} else {
		in["max_tokens"] = m.maxTokens
	}
	var messages []message
	if req.system != "" {
		messages = append(messages, message{"system", req.system})
	}
	for _, t := range req.turns {
		messages = append(messages, message{t.role, t.text})
	}
	in["messages"] = messages
	var out struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
				Refusal *string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	headers := map[string]string{}
	if m.key != "" { // a local server may not need one
		headers["Authorization"] = "Bearer " + m.key
	}
	if err := m.post(ctx, strings.TrimRight(m.base, "/")+"/chat/completions", headers, in, &out); err != nil {
		return response{}, err
	}
	if len(out.Choices) == 0 {
		return response{}, fmt.Errorf("%s: response without choices", m.provider)
	}
	choice := out.Choices[0]
	r := response{stop: choice.FinishReason, inputTokens: out.Usage.PromptTokens, outputTokens: out.Usage.CompletionTokens}
	if choice.Message.Content != nil {
		r.text = *choice.Message.Content
	}
	if choice.Message.Refusal != nil && *choice.Message.Refusal != "" {
		r.stop = "refusal"
	}
	return r, nil
}

// fakeModel answers offline by applying mangle to every token of the last user turn. It is how the harness
// tests itself, and what it runs when no provider is chosen.
type fakeModel struct {
	behavior string
	mangle   func(token string) string
}

func (m fakeModel) name() string { return "simulated/" + m.behavior }

func (m fakeModel) complete(_ context.Context, req request) (response, error) {
	last := req.turns[len(req.turns)-1].text
	if len(req.turns) > 1 {
		last = req.turns[0].text // a question about earlier turns: answer from them
	}
	var parts []string
	for _, tok := range tokenPattern.FindAllString(last, -1) {
		if s := m.mangle(tok); s != "" {
			parts = append(parts, s)
		}
	}
	return response{text: "Here you go: " + strings.Join(parts, ", ") + ".", stop: "stop"}, nil
}

// fakeModels returns one simulated model per way of breaking a token, so that a run offline shows every
// outcome the scoring can report.
func fakeModels() []model {
	inner := func(tok string) string { return tok[1 : len(tok)-1] }
	return []model{
		fakeModel{"perfect", func(tok string) string { return tok }},
		fakeModel{"uppercase", strings.ToUpper},
		fakeModel{"html_escaped", func(tok string) string { return "&lt;" + inner(tok) + "&gt;" }},
		fakeModel{"spaced", func(tok string) string { return "< " + inner(tok) + " >" }},
		fakeModel{"new_separator", func(tok string) string { return "<" + strings.Replace(inner(tok), "_", "-", 1) + ">" }},
		fakeModel{"square_brackets", func(tok string) string { return "[" + inner(tok) + "]" }},
		fakeModel{"typo_in_id", func(tok string) string { return tok[:len(tok)-2] + ">" }},
		fakeModel{"drops_tokens", func(string) string { return "" }},
		fakeModel{"invents_tokens", func(tok string) string { return tok + " <CPF_0000000000>" }},
		fakeModel{"repeats_and_mangles", func(tok string) string { return tok + " " + strings.ToUpper(tok) }},
	}
}
