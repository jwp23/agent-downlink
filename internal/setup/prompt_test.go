package setup

import (
	"bytes"
	"strings"
	"testing"
)

func TestAskShowsTheDefaultAndReturnsItOnEnter(t *testing.T) {
	cases := map[string]struct {
		question, def, typed   string
		wantPrompt, wantAnswer string
	}{
		"default shown and taken":      {"Machine name", "workstation", "", "Machine name [workstation]: ", "workstation"},
		"default shown and overridden": {"Machine name", "workstation", "laptop", "Machine name [workstation]: ", "laptop"},
		"no default":                   {"Bucket", "", "scratch-bucket", "Bucket: ", "scratch-bucket"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader(tc.typed+"\n"), &out)
			got, err := p.Ask(tc.question, tc.def)
			if err != nil || got != tc.wantAnswer {
				t.Errorf("Ask = %q, %v; want %q", got, err, tc.wantAnswer)
			}
			if out.String() != tc.wantPrompt {
				t.Errorf("prompt = %q, want %q", out.String(), tc.wantPrompt)
			}
		})
	}
}
