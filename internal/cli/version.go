package cli

import (
	"fmt"

	"github.com/shurastik/agterm-kit/internal/buildinfo"
)

// Version prints the build version.
func (e *Env) Version(args []string) int {
	if len(args) > 0 {
		return e.errorf("version takes no arguments")
	}
	fmt.Fprintln(e.Stdout, buildinfo.Version)
	return ExitOK
}
