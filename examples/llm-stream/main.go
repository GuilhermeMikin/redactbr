// Command llm-stream simulates an LLM call protected by a RedactBR session. The prompt is tokenized before
// it would leave the process; a fake model answers with the tokens it received, streamed in small chunks
// that split tokens apart; RestoreWriter restores the answer as it streams. No model or network is used,
// and the identifiers are generated test values, not real ones.
package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/GuilhermeMikin/redactbr"
)

// The user also pastes something that looks like a token. The session reserves it, so it can never be
// restored into a value, even if the model repeats it.
const prompt = "Hi! My CPF is 123.456.789-09 and my email is ana.silva@example.com. " +
	"Please confirm my registration. An old message mentioned <CPF_aaaaaaaaaa>, what is that?"

func main() {
	r, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	session := r.NewSession() // one session per conversation

	redacted, err := session.Redact(prompt)
	if err != nil {
		log.Fatal(err) // fail closed: send nothing
	}
	fmt.Printf("Prompt sent to the model:\n  %s\n\n", redacted)

	chunks := fakeModel(redacted, 7)
	fmt.Printf("Model stream, %d chunks of up to 7 bytes:\n  %s\n\n", len(chunks), strings.Join(quote(chunks), " "))

	fmt.Println("Restored stream:")
	fmt.Print("  ")
	w := session.RestoreWriter(os.Stdout)
	for _, c := range chunks {
		if _, err := w.Write([]byte(c)); err != nil {
			log.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	fmt.Println()
}

var tokenPattern = regexp.MustCompile(`<(CPF|CNPJ|EMAIL)_[0-9a-z]{10}>`)

// fakeModel answers by copying the tokens it was given, as a model is asked to, and splits the answer into
// chunks of size bytes, deterministically.
func fakeModel(prompt string, size int) []string {
	toks := tokenPattern.FindAllString(prompt, -1) // CPF token, email token, pasted look-alike
	if len(toks) != 3 {
		log.Fatalf("unexpected prompt: %d tokens", len(toks))
	}
	answer := fmt.Sprintf("Done! The registration for CPF %s is confirmed and a receipt was sent to %s. "+
		"I don't know what %s refers to.", toks[0], toks[1], toks[2])
	var chunks []string
	for i := 0; i < len(answer); i += size {
		chunks = append(chunks, answer[i:min(i+size, len(answer))])
	}
	return chunks
}

func quote(chunks []string) []string {
	q := make([]string, len(chunks))
	for i, c := range chunks {
		q[i] = fmt.Sprintf("%q", c)
	}
	return q
}
