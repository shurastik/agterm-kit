// Package agtermtest provides a recording fake of agterm.Ctl.
package agtermtest

import (
	"context"
	"fmt"
	"sync"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

// Call is one recorded Ctl method call; Args holds its non-context arguments.
type Call struct {
	Method string
	Args   []string
}

func (c Call) String() string { return fmt.Sprintf("%s%q", c.Method, c.Args) }

// Fake records every call. Set the result fields to script responses.
type Fake struct {
	mu    sync.Mutex
	Calls []Call

	CompanionID  string
	CompanionErr error
	StatusErr    error
	InfoErr      error
	Outcome      agterm.Outcome
	OverlayErr   error
	// OnOverlay, when set, runs before OverlayHTML returns (e.g. to read the page file).
	OnOverlay func(file string)
}

var _ agterm.Ctl = (*Fake)(nil)

func (f *Fake) record(method string, args ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: method, Args: args})
}

// Methods returns the recorded method names in order.
func (f *Fake) Methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.Calls))
	for i, c := range f.Calls {
		out[i] = c.Method
	}
	return out
}

func (f *Fake) NewCompanion(_ context.Context, parent, name, command, cwd string) (string, error) {
	f.record("NewCompanion", parent, name, command, cwd)
	return f.CompanionID, f.CompanionErr
}

func (f *Fake) Status(_ context.Context, target, state string) error {
	f.record("Status", target, state)
	return f.StatusErr
}

func (f *Fake) Info(_ context.Context, target, markdown string) error {
	f.record("Info", target, markdown)
	return f.InfoErr
}

func (f *Fake) OverlayHTML(_ context.Context, target, file string) (agterm.Outcome, error) {
	f.record("OverlayHTML", target, file)
	if f.OnOverlay != nil {
		f.OnOverlay(file)
	}
	return f.Outcome, f.OverlayErr
}
