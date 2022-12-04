package filecoin

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	whypfs "github.com/application-research/whypfs-core"
	"github.com/dustin/go-humanize"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/big"
	"golang.org/x/term"
)

type RetrievalStats interface {
	GetByteSize() uint64
	GetDuration() time.Duration
	GetAverageBytesPerSecond() uint64
}

// Takes a list of network configs to attempt to retrieve from, in order. Valid
// structs for the interface: IPFSRetrievalConfig, FILRetrievalConfig
func RetrieveFromBestCandidate(
	node *whypfs.Node,
	ctx context.Context,
	attempts []GetAttempt,
) (RetrievalStats, error) {
	for _, attempt := range attempts {
		stats, err := attempt.Retrieve(ctx, node)
		if err == nil {
			return stats, nil
		}
	}

	return nil, fmt.Errorf("all retrieval attempts failed")
}

func totalCost(qres *retrievalmarket.QueryResponse) big.Int {
	return big.Add(big.Mul(qres.MinPricePerByte, big.NewIntUnsigned(qres.Size)), qres.UnsealPrice)
}

func printProgress(bytesReceived uint64) {
	str := fmt.Sprintf("%v (%v)", bytesReceived, humanize.IBytes(bytesReceived))

	termWidth, _, err := term.GetSize(int(os.Stdin.Fd()))
	strLen := len(str)
	if err == nil {

		if strLen < termWidth {
			// If the string is shorter than the terminal width, pad right side
			// with spaces to remove old text
			str = strings.Join([]string{str, strings.Repeat(" ", termWidth-strLen)}, "")
		} else if strLen > termWidth {
			// If the string doesn't fit in the terminal, cut it down to a size
			// that fits
			str = str[:termWidth]
		}
	}

	fmt.Fprintf(os.Stderr, "%s\r", str)
}
