// Package agtermtest provides a recording fake of agterm.Ctl.
package agtermtest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
	// CtxErrs[i] is ctx.Err() at the time of Calls[i].
	CtxErrs []error

	CompanionID  string
	CompanionErr error
	StatusErr    error
	InfoErr      error
	Outcome      agterm.Outcome
	OverlayErr   error
	WaitStatus   string
	WaitErr      error
	CloseErr     error
	// OnStatus, when set, decides each Status call's error (overrides StatusErr).
	OnStatus func(state string) error
	// OnWait, when set, runs before Wait returns (e.g. to write result.json).
	OnWait func()
	// OnNewCompanion, when set, runs before NewCompanion returns.
	OnNewCompanion func()
	// OnOverlay, when set, runs before OverlayHTML returns (e.g. to read the page file).
	OnOverlay func(file string)
}

var _ agterm.Ctl = (*Fake)(nil)

func (f *Fake) record(ctx context.Context, method string, args ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: method, Args: args})
	f.CtxErrs = append(f.CtxErrs, ctx.Err())
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

func (f *Fake) NewCompanion(ctx context.Context, parent, name, command, cwd string) (string, error) {
	f.record(ctx, "NewCompanion", parent, name, command, cwd)
	if f.OnNewCompanion != nil {
		f.OnNewCompanion()
	}
	return f.CompanionID, f.CompanionErr
}

func (f *Fake) Status(ctx context.Context, target, state string) error {
	f.record(ctx, "Status", target, state)
	if f.OnStatus != nil {
		return f.OnStatus(state)
	}
	return f.StatusErr
}

func (f *Fake) Info(ctx context.Context, target, markdown string) error {
	f.record(ctx, "Info", target, markdown)
	return f.InfoErr
}

func (f *Fake) OverlayHTML(ctx context.Context, target, file string) (agterm.Outcome, error) {
	f.record(ctx, "OverlayHTML", target, file)
	if f.OnOverlay != nil {
		f.OnOverlay(file)
	}
	return f.Outcome, f.OverlayErr
}

// Wait records target, the joined statuses and the timeout.
func (f *Fake) Wait(ctx context.Context, target string, until []string, timeout time.Duration) (string, error) {
	f.record(ctx, "Wait", target, strings.Join(until, ","), timeout.String())
	if f.OnWait != nil {
		f.OnWait()
	}
	return f.WaitStatus, f.WaitErr
}

func (f *Fake) Close(ctx context.Context, target string) error {
	f.record(ctx, "Close", target)
	return f.CloseErr
}
