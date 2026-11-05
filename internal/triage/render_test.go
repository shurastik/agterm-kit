package triage

import (
	"regexp"
	"strings"
	"testing"

	"github.com/shurastik/agterm-kit/internal/review"
)

func sample() review.Review {
	return review.Review{
		Verdict: "needs-attention",
		Summary: "Two issues & one nit.",
		Findings: []review.Finding{
			{Severity: "high", Title: "Race on cache map", Body: "Writes from two goroutines.", File: "internal/cache.go",
				LineStart: 40, LineEnd: 52, Confidence: 0.85, Recommendation: "Guard with sync.Mutex."},
			{Severity: "low", Title: "Typo in log", Body: "recieve", File: "main.go",
				LineStart: 7, LineEnd: 7, Confidence: 0.4, Recommendation: "Spell receive."},
		},
	}
}

func render(t *testing.T, r review.Review) string {
	t.Helper()
	b, err := Render(r, "n0nce42")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRenderShowsAllFields(t *testing.T) {
	page := render(t, sample())
	for _, want := range []string{
		"Two issues &amp; one nit.",
		"Race on cache map", "Writes from two goroutines.", "internal/cache.go:40-52", "85%", "Guard with sync.Mutex.",
		"Typo in log", "main.go:7", "40%",
		">#1<", ">#2<", ">high<", ">low<",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, "main.go:7-7") {
		t.Error("single-line range should render as file:line")
	}
}

func checked(page, name, value string) bool {
	re := regexp.MustCompile(`<input[^>]*name="` + name + `"[^>]*value="` + value + `"[^>]*>`)
	m := re.FindString(page)
	return strings.Contains(m, " checked")
}

func TestRenderDefaultsBySeverity(t *testing.T) {
	r := sample()
	r.Findings = append(r.Findings,
		review.Finding{Severity: "critical", Title: "c", Body: "b", File: "f", LineStart: 1, LineEnd: 1},
		review.Finding{Severity: "medium", Title: "m", Body: "b", File: "f", LineStart: 1, LineEnd: 1})
	page := render(t, r)
	cases := []struct{ n, want string }{{"1", "fix"}, {"2", "skip"}, {"3", "fix"}, {"4", "fix"}}
	for _, c := range cases {
		for _, v := range []string{"fix", "skip", "discuss"} {
			if got := checked(page, "d-"+c.n, v); got != (v == c.want) {
				t.Errorf("finding %s %s checked=%v, want default %s", c.n, v, got, c.want)
			}
		}
		if !strings.Contains(page, `name="note-`+c.n+`"`) {
			t.Errorf("finding %s has no note field", c.n)
		}
	}
}

func TestRenderCSPAndScript(t *testing.T) {
	page := render(t, sample())
	csp := `default-src 'none'; script-src 'nonce-n0nce42'; style-src 'unsafe-inline'; img-src 'none'; form-action 'none'`
	if !strings.Contains(page, `<meta http-equiv="Content-Security-Policy" content="`+csp+`">`) {
		t.Fatal("CSP meta missing or altered")
	}
	scripts := regexp.MustCompile(`(?is)<script\b[^>]*>(.*?)</script>`).FindAllStringSubmatch(page, -1)
	if len(scripts) != 1 {
		t.Fatalf("want exactly one script, got %d", len(scripts))
	}
	if !strings.Contains(scripts[0][0], `<script nonce="n0nce42">`) {
		t.Fatalf("script lacks nonce: %.80s", scripts[0][0])
	}
	calls := regexp.MustCompile(`agterm\.request\(\s*"([^"]+)"`).FindAllStringSubmatch(scripts[0][1], -1)
	if len(calls) != 1 || calls[0][1] != "session.overlay.submit" {
		t.Fatalf("bridge calls = %v, want only session.overlay.submit", calls)
	}
	if strings.Contains(page, "data-agterm") {
		t.Fatal("no data-agterm attributes allowed")
	}
	if regexp.MustCompile(`(?i)\son[a-z]+=`).MatchString(page) {
		t.Fatal("no inline event handlers allowed")
	}
}

func TestRenderEscapesHostileText(t *testing.T) {
	r := sample()
	r.Summary = `</title><script>alert(1)</script>`
	r.Findings[0].Title = `<script>agterm.request("session.close")</script>`
	r.Findings[0].Body = `" onmouseover="x` + "\n" + `<img src=https://example.com/x>`
	r.Findings[0].File = `"><svg onload=x>`
	r.Findings[0].Recommendation = `<a href="javascript:x">click</a>`
	page := render(t, r)

	if n := strings.Count(strings.ToLower(page), "<script"); n != 1 {
		t.Fatalf("<script count = %d, want 1 (ours)", n)
	}
	for _, bad := range []string{`<img`, `<svg`, `<a href`, `" onmouseover="`, `session.close")</script>`, `onload=x>`} {
		if strings.Contains(page, bad) {
			t.Errorf("hostile fragment %q rendered raw", bad)
		}
	}
	for _, want := range []string{`&lt;script&gt;agterm.request(&#34;session.close&#34;)&lt;/script&gt;`, `&lt;img src=https://example.com/x&gt;`} {
		if !strings.Contains(page, want) {
			t.Errorf("escaped text %q not shown", want)
		}
	}
}

func TestNewNonce(t *testing.T) {
	a, b := NewNonce(), NewNonce()
	if a == b || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(a) {
		t.Fatalf("nonces %q %q", a, b)
	}
}
