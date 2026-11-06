// Command tokenfidelity measures whether LLMs copy RedactBR session tokens faithfully: it tokenizes a fixed set
// of generated texts, asks a model to work with them, and scores what happened to each token in the answer.
//
// With no flags it runs offline against simulated models, which tests the harness and says nothing about real
// models. A real provider needs -provider and -model; without -live it only prints the plan. Nothing reads a
// key or opens a connection before -live. See docs/token-fidelity.md.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GuilhermeMikin/redactbr"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

type options struct {
	provider, modelID, base, out string
	repeats, maxTokens, cap      int
	live, byScenario             bool
	timeout                      time.Duration
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("tokenfidelity", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.provider, "provider", "simulated", "simulated (offline), anthropic, or openai (also OpenAI-compatible servers, with -base-url)")
	fs.StringVar(&o.modelID, "model", "", "model id; required for a real provider")
	fs.StringVar(&o.base, "base-url", "", "API base URL, to override the provider's")
	fs.IntVar(&o.repeats, "repeats", 3, "runs of each scenario; each run draws new token ids")
	fs.IntVar(&o.maxTokens, "max-tokens", 4096, "output token limit per request; thinking counts towards it on models that think")
	fs.IntVar(&o.cap, "max-requests", 100, "refuse to run more requests than this")
	fs.BoolVar(&o.live, "live", false, "send requests to the provider, which may cost money; without it, only print the plan")
	fs.BoolVar(&o.byScenario, "by-scenario", false, "also report each scenario (always on for a real provider)")
	fs.StringVar(&o.out, "out", "", "write one JSON object per request to this file (tokens and answers, no restored values)")
	fs.DurationVar(&o.timeout, "timeout", 3*time.Minute, "time limit per request")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	switch {
	case fs.NArg() > 0:
		return o, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	case o.repeats < 1 || o.maxTokens < 1 || o.cap < 1:
		return o, errors.New("-repeats, -max-tokens and -max-requests must be positive")
	case o.provider != "simulated" && o.provider != "anthropic" && o.provider != "openai":
		return o, fmt.Errorf("unknown provider %q", o.provider)
	case o.provider != "simulated" && o.modelID == "":
		return o, errors.New("-model is required for a real provider")
	case o.provider == "simulated" && (o.live || o.modelID != "" || o.base != ""):
		return o, errors.New("-live, -model and -base-url need -provider")
	}
	return o, nil
}

// keyVariable names the environment variable with the provider's key.
func keyVariable(provider string) string {
	if provider == "anthropic" {
		return "ANTHROPIC_API_KEY"
	}
	return "OPENAI_API_KEY"
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	o, err := parseFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "tokenfidelity:", err)
		return 2
	}
	requests := len(scenarios) * len(variants) * o.repeats
	if requests > o.cap {
		fmt.Fprintf(stderr, "tokenfidelity: %d requests is more than -max-requests %d\n", requests, o.cap)
		return 2
	}

	var models []model
	if o.provider == "simulated" {
		models = fakeModels()
		fmt.Fprintln(stdout, "SIMULATED: these numbers come from scripted fake models. They test this harness and say nothing about real models.")
		fmt.Fprintln(stdout)
	} else {
		base := o.base
		if base == "" {
			base = anthropicDefaultBase
			if o.provider == "openai" {
				base = openAIDefaultBase
			}
		}
		printPlan(stdout, o, base, requests)
		if !o.live {
			fmt.Fprintln(stdout, "Nothing was sent. Add -live to run this plan.")
			return 0
		}
		key := getenv(keyVariable(o.provider))
		if key == "" && (o.provider == "anthropic" || o.base == "") {
			fmt.Fprintf(stderr, "tokenfidelity: %s is not set\n", keyVariable(o.provider))
			return 1
		}
		if err := checkEndpoint(base, key); err != nil {
			fmt.Fprintln(stderr, "tokenfidelity:", err)
			return 2
		}
		hm := httpModel{provider: o.provider, id: o.modelID, base: base, key: key, maxTokens: o.maxTokens, client: newHTTPClient(o.timeout)}
		if o.provider == "anthropic" {
			hm.workspace = getenv("ANTHROPIC_WORKSPACE_ID")
			models = []model{anthropicModel{hm}}
		} else {
			models = []model{openAIModel{hm}}
		}
	}

	out := io.Discard
	if o.out != "" {
		f, err := os.Create(o.out)
		if err != nil {
			fmt.Fprintln(stderr, "tokenfidelity:", err)
			return 1
		}
		defer func() { _ = f.Close() }()
		out = f
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false) // keep tokens readable
	if err := enc.Encode(metadata(o)); err != nil {
		fmt.Fprintln(stderr, "tokenfidelity:", err)
		return 1
	}

	var results []result
	for _, m := range models {
		for _, sc := range scenarios {
			for _, v := range variants {
				for repeat := range o.repeats {
					res, err := runCase(ctx, m, sc, v, repeat)
					if err != nil { // the harness itself is broken: stop before sending anything else
						fmt.Fprintln(stderr, "tokenfidelity:", err)
						return 1
					}
					results = append(results, res)
					if err := enc.Encode(res); err != nil {
						fmt.Fprintln(stderr, "tokenfidelity:", err)
						return 1
					}
				}
			}
		}
	}
	report(stdout, results, o.byScenario || o.provider != "simulated")
	return 0
}

// A variant is the system prompt a scenario runs with.
type variant struct{ name, system string }

var variants = []variant{{"no_instruction", ""}, {"instruction", systemInstruction}}

func printPlan(w io.Writer, o options, base string, requests int) {
	input := 0
	for _, sc := range scenarios {
		for _, t := range sc.turns {
			input += len(t.text)
		}
	}
	fmt.Fprintf(w, "Plan: %s, model %s, at %s\n", o.provider, o.modelID, base)
	fmt.Fprintf(w, "  requests: %d (%d scenarios x %d variants x %d repeats), up to 3 attempts each on rate limits and server errors\n",
		requests, len(scenarios), len(variants), o.repeats)
	fmt.Fprintf(w, "  output: at most %d tokens per request, %d in total\n", o.maxTokens, requests*o.maxTokens)
	fmt.Fprintf(w, "  input: about %d bytes per round of scenarios, plus %d bytes of instruction per request that has it\n", input, len(systemInstruction))
	fmt.Fprintf(w, "  key: %s, read only with -live\n", keyVariable(o.provider))
}

type runMetadata struct {
	Harness struct {
		Scenarios string    `json:"scenarios_sha256"`
		Go        string    `json:"go"`
		Started   time.Time `json:"started"`
		Provider  string    `json:"provider"`
		Model     string    `json:"model"`
		Repeats   int       `json:"repeats"`
		MaxTokens int       `json:"max_tokens"`
		Version   string    `json:"anthropic_version,omitempty"`
	} `json:"harness"`
}

func metadata(o options) runMetadata {
	var m runMetadata
	h := sha256.New()
	fmt.Fprintf(h, "%q %q", systemInstruction, fmt.Sprint(scenarios))
	m.Harness.Scenarios = fmt.Sprintf("%x", h.Sum(nil))
	m.Harness.Go = runtime.Version()
	m.Harness.Started = time.Now().UTC()
	m.Harness.Provider, m.Harness.Model = o.provider, o.modelID
	m.Harness.Repeats, m.Harness.MaxTokens = o.repeats, o.maxTokens
	if o.provider == "anthropic" {
		m.Harness.Version = anthropicVersion
	}
	return m
}

// A tokenScore is what happened to one token in one answer. Expected is false for a token the scenario did not
// ask for, which only counts if it shows up: then a model used the wrong one.
type tokenScore struct {
	Token    string  `json:"token"`
	Expected bool    `json:"expected"`
	Outcome  outcome `json:"outcome"`
}

type result struct {
	Model          string       `json:"model"`
	Scenario       string       `json:"scenario"`
	Variant        string       `json:"variant"`
	Repeat         int          `json:"repeat"`
	Stress         bool         `json:"stress"`
	Answer         string       `json:"answer"`
	Stop           string       `json:"stop,omitempty"`
	Tokens         []tokenScore `json:"tokens"`
	Invented       []string     `json:"invented,omitempty"`
	RawValue       bool         `json:"raw_value_in_answer"` // a value appeared before restoration
	Restored       bool         `json:"all_values_restored"`
	StreamAgrees   bool         `json:"stream_agrees"` // RestoreWriter gave the same text as Restore
	InputTokens    int          `json:"input_tokens,omitempty"`
	OutputTokens   int          `json:"output_tokens,omitempty"`
	ProviderFailed string       `json:"error,omitempty"`
}

// runCase runs one scenario once. An error means the harness cannot trust its own setup; a provider failure is
// recorded in the result instead, so that one bad request does not lose a paid run.
func runCase(ctx context.Context, m model, sc scenario, v variant, repeat int) (result, error) {
	r, err := redactbr.New()
	if err != nil {
		return result{}, err
	}
	session := r.NewSession()
	req := request{}
	if v.system != "" {
		if req.system, err = session.Redact(v.system); err != nil {
			return result{}, err
		}
	}
	all := make(map[string]bool) // every value in the scenario
	for _, t := range sc.turns {
		text, err := session.Redact(t.text)
		if err != nil {
			return result{}, err
		}
		req.turns = append(req.turns, turn{t.role, text})
	}
	for _, value := range []string{cpfAna, cpfBruno, cpfCarla, cnpjDelta, cnpjBeta, mailAna, mailBruno} {
		for _, t := range req.turns {
			if strings.Contains(t.text, value) {
				return result{}, fmt.Errorf("scenario %s: a value was left in the text to send", sc.name)
			}
		}
		for _, t := range sc.turns {
			if strings.Contains(t.text, value) {
				all[value] = true
			}
		}
	}

	res := result{Model: m.name(), Scenario: sc.name, Variant: v.name, Repeat: repeat, Stress: sc.stress}
	resp, err := m.complete(ctx, req)
	if err != nil {
		res.ProviderFailed = err.Error()
		return res, nil
	}
	res.Answer, res.Stop, res.InputTokens, res.OutputTokens = resp.text, resp.stop, resp.inputTokens, resp.outputTokens

	issued := make(map[string]bool)
	for value := range all {
		tok, err := session.Redact(value) // the same token as in the text: same session, same value
		if err != nil {
			return result{}, err
		}
		if !tokenPattern.MatchString(tok) {
			return result{}, fmt.Errorf("scenario %s: a value did not become a token", sc.name)
		}
		issued[tok] = true
		res.Tokens = append(res.Tokens, tokenScore{Token: tok, Expected: slices.Contains(sc.values, value), Outcome: classify(resp.text, tok)})
		res.RawValue = res.RawValue || strings.Contains(resp.text, value)
	}
	slices.SortFunc(res.Tokens, func(a, b tokenScore) int { return strings.Compare(a.Token, b.Token) })
	res.Invented = invented(resp.text, issued)

	restored := session.Restore(resp.text)
	res.Restored = true
	for _, value := range sc.values {
		res.Restored = res.Restored && strings.Contains(restored, value)
	}
	var stream bytes.Buffer
	w := session.RestoreWriter(&stream)
	for text := resp.text; len(text) > 0; {
		n := min(7, len(text))
		if _, err := w.Write([]byte(text[:n])); err != nil {
			return result{}, err
		}
		text = text[n:]
	}
	if err := w.Close(); err != nil {
		return result{}, err
	}
	res.StreamAgrees = stream.String() == restored
	return res, nil
}

// tally adds up results for one line of a report.
type tally struct {
	requests, failed, refused, truncated, complete int
	tokens, invented, wrongToken, rawValues        int
	streamDisagrees                                int
	outcomes                                       map[outcome]int
}

func (t *tally) add(r result) {
	t.requests++
	if r.ProviderFailed != "" {
		t.failed++
		return
	}
	switch r.Stop {
	case "refusal":
		t.refused++
	case "max_tokens", "length":
		t.truncated++
	}
	if t.outcomes == nil {
		t.outcomes = make(map[outcome]int)
	}
	for _, s := range r.Tokens {
		switch {
		case s.Expected:
			t.tokens++
			t.outcomes[s.Outcome]++
		case s.Outcome != outcomeMissing:
			t.wrongToken++
		}
	}
	t.invented += len(r.Invented)
	if r.RawValue {
		t.rawValues++
	}
	if r.Restored {
		t.complete++
	}
	if !r.StreamAgrees {
		t.streamDisagrees++
	}
}

// A reportKey is one line of a report; group is "ordinary" or "stress", or the name of a scenario.
type reportKey struct{ model, variant, group string }

// report prints what reached the reader, per model and variant, with stress scenarios apart.
func report(w io.Writer, results []result, byScenario bool) {
	writeTable(w, "By model and variant", "group", results, func(r result) string {
		if r.Stress {
			return "stress"
		}
		return "ordinary"
	})
	if byScenario {
		fmt.Fprintln(w)
		writeTable(w, "By scenario", "scenario", results, func(r result) string { return r.Scenario })
	}

	var totalIn, totalOut int
	var firstError string
	for _, r := range results {
		totalIn += r.InputTokens
		totalOut += r.OutputTokens
		if firstError == "" {
			firstError = r.ProviderFailed
		}
	}
	if firstError != "" {
		fmt.Fprintf(w, "\nFirst provider error: %s\n", firstError)
	}
	if totalIn+totalOut > 0 {
		fmt.Fprintf(w, "\nTokens used, as the provider reported them: %d input, %d output.\n", totalIn, totalOut)
	}
}

func writeTable(w io.Writer, title, column string, results []result, group func(result) string) {
	tallies := make(map[reportKey]*tally)
	var order []reportKey
	for _, r := range results {
		k := reportKey{r.Model, r.Variant, group(r)}
		if tallies[k] == nil {
			tallies[k] = &tally{}
			order = append(order, k)
		}
		tallies[k].add(r)
	}
	fmt.Fprintln(w, title)
	tw := tabwriter.NewWriter(w, 2, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "model\tvariant\t%s\trequests\tcomplete\ttokens\trestorable\tchanged\tmissing\tinvented\twrong_token\traw_value\tfailed\trefused\ttruncated\tstream_diff\n", column)
	for _, k := range order {
		t := tallies[k]
		restorable := t.outcomes[outcomeExact] + t.outcomes[outcomeMixed]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			k.model, k.variant, k.group, t.requests, t.complete, t.tokens, percent(restorable, t.tokens), changedSummary(t),
			t.outcomes[outcomeMissing], t.invented, t.wrongToken, t.rawValues, t.failed, t.refused, t.truncated, t.streamDisagrees)
	}
	_ = tw.Flush()
}

func percent(n, of int) string {
	if of == 0 {
		return "-"
	}
	return fmt.Sprintf("%d (%.1f%%)", n, 100*float64(n)/float64(of))
}

// changedSummary lists the ways tokens were changed, with counts.
func changedSummary(t *tally) string {
	var parts []string
	for _, o := range allOutcomes {
		if o != outcomeExact && o != outcomeMissing && t.outcomes[o] > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", o, t.outcomes[o]))
		}
	}
	if len(parts) == 0 {
		return "0"
	}
	return strings.Join(parts, " ")
}
