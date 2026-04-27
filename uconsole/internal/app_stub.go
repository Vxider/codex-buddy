//go:build !uconsole_gui

package uconsole

import (
	"context"
	"fmt"
	"log"

	"github.com/vxider/codex-buddy/internal/config"
)

func Run(_ context.Context, _ config.Config, _ string, _ *log.Logger) error {
	return fmt.Errorf("uconsole GUI is not built in this binary; rebuild with `-tags uconsole_gui`")
}
