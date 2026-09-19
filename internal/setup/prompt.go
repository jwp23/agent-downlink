// Package setup is the interactive setup and add-machine flows.
package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Prompter asks the operator questions. Secrets are read without echo when input is a
// terminal, and as a plain line otherwise, so tests and pipes can supply them.
type Prompter struct {
	in       *bufio.Reader
	out      io.Writer
	terminal *os.File // nil when input is not a terminal
}

// NewPrompter asks its questions on out and reads the answers from in.
func NewPrompter(in io.Reader, out io.Writer) *Prompter {
	p := &Prompter{in: bufio.NewReader(in), out: out}
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		p.terminal = f
	}
	return p
}

// Ask returns the operator's answer, or def when they just press Enter.
func (p *Prompter) Ask(question, def string) (string, error) {
	if def != "" {
		_, _ = fmt.Fprintf(p.out, "%s [%s]: ", question, def)
	} else {
		_, _ = fmt.Fprintf(p.out, "%s: ", question)
	}
	answer, err := p.line()
	if err != nil {
		return "", err
	}
	if answer == "" {
		return def, nil
	}
	return answer, nil
}

// AskSecret reads an answer that must not be echoed.
func (p *Prompter) AskSecret(question string) (string, error) {
	_, _ = fmt.Fprintf(p.out, "%s: ", question)
	if p.terminal == nil {
		return p.line()
	}
	b, err := term.ReadPassword(int(p.terminal.Fd()))
	_, _ = fmt.Fprintln(p.out)
	return strings.TrimSpace(string(b)), err
}

// Confirm is true only for the whole word "yes", so a stray Enter never overwrites anything.
func (p *Prompter) Confirm(question string) (bool, error) {
	answer, err := p.Ask(question+" Type yes to continue", "")
	return answer == "yes", err
}

func (p *Prompter) line() (string, error) {
	s, err := p.in.ReadString('\n')
	if errors.Is(err, io.EOF) && s == "" {
		return "", errors.New("input ended before every question was answered")
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(s), nil
}
