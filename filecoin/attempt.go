package filecoin

import (
	"context"

	whypfs "github.com/application-research/whypfs-core"
)

// A get attempt is a configuration for performing a specific fetch
// over a specific network
type GetAttempt interface {
	Retrieve(context.Context, *whypfs.Node) (RetrievalStats, error)
}
