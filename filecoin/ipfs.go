package filecoin

import (
	"context"
	"fmt"
	"sync"
	"time"

	whypfs "github.com/application-research/whypfs-core"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/labstack/gommon/log"
)

type IPFSRetrievalStats struct {
	ByteSize uint64
	Duration time.Duration
}

func (stats *IPFSRetrievalStats) GetByteSize() uint64 {
	return stats.ByteSize
}

func (stats *IPFSRetrievalStats) GetDuration() time.Duration {
	return stats.Duration
}

func (stats *IPFSRetrievalStats) GetAverageBytesPerSecond() uint64 {
	return uint64(float64(stats.ByteSize) / stats.Duration.Seconds())
}

type IPFSRetrievalAttempt struct {
	Cid cid.Cid
}

func (attempt *IPFSRetrievalAttempt) Retrieve(ctx context.Context, node *whypfs.Node) (RetrievalStats, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Info("Searching IPFS for CID...")

	providers := node.Dht.FindProvidersAsync(ctx, attempt.Cid, 0)

	// Ready will be true if we connected to at least one provider, false if no
	// miners successfully connected
	ready := make(chan bool, 1)
	go func() {
		for {
			select {
			case provider, ok := <-providers:
				if !ok {
					ready <- false
					return
				}

				// If no addresses are listed for the provider, we should just
				// skip it
				if len(provider.Addrs) == 0 {
					log.Debugf("Skipping IPFS provider with no addresses %s", provider.ID)
					continue
				}

				log.Infof("Connected to IPFS provider %s", provider.ID)
				ready <- true
			case <-ctx.Done():
				return
			}
		}
	}()

	select {
	// TODO: also add connection timeout
	case <-ctx.Done():
		return nil, ctx.Err()
	case ready := <-ready:
		if !ready {
			return nil, fmt.Errorf("couldn't find CID")
		}
	}

	// If we were able to connect to at least one of the providers, go ahead
	// with the retrieval

	var progressLk sync.Mutex
	var bytesRetrieved uint64 = 0
	startTime := time.Now()

	log.Info("Starting retrieval")

	bserv := blockservice.New(node.Blockstore, node.Bitswap)
	dserv := merkledag.NewDAGService(bserv)

	cset := cid.NewSet()
	if err := merkledag.Walk(ctx, func(ctx context.Context, c cid.Cid) ([]*ipldformat.Link, error) {
		node, err := dserv.Get(ctx, c)
		if err != nil {
			return nil, err
		}

		// Only count leaf nodes toward the total size
		if len(node.Links()) == 0 {
			progressLk.Lock()
			nodeSize, err := node.Size()
			if err != nil {
				nodeSize = 0
			}
			bytesRetrieved += nodeSize
			printProgress(bytesRetrieved)
			progressLk.Unlock()
		}

		if c.Type() == cid.Raw {
			return nil, nil
		}

		return node.Links(), nil
	}, attempt.Cid, cset.Visit, merkledag.Concurrent()); err != nil {
		return nil, err
	}

	log.Info("IPFS retrieval succeeded")

	return &IPFSRetrievalStats{
		ByteSize: bytesRetrieved,
		Duration: time.Since(startTime),
	}, nil
}
