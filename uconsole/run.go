package uconsole

import (
	"context"
	"log"

	"github.com/vxider/codex-buddy/internal/config"
	internaluconsole "github.com/vxider/codex-buddy/uconsole/internal"
)

func Run(ctx context.Context, cfg config.Config, configPath string, logger *log.Logger) error {
	return internaluconsole.Run(ctx, cfg, configPath, logger)
}
