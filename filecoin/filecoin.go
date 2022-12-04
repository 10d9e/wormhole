package filecoin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/application-research/filclient"
	"github.com/application-research/filclient/retrievehelper"
	whypfs "github.com/application-research/whypfs-core"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/labstack/gommon/log"
	"golang.org/x/xerrors"
)

type FILRetrievalStats struct {
	filclient.RetrievalStats
}

func (stats *FILRetrievalStats) GetByteSize() uint64 {
	return stats.Size
}

func (stats *FILRetrievalStats) GetDuration() time.Duration {
	return stats.Duration
}

func (stats *FILRetrievalStats) GetAverageBytesPerSecond() uint64 {
	return stats.AverageSpeed
}

type FILRetrievalAttempt struct {
	FilClient  *filclient.FilClient
	Cid        cid.Cid
	Candidates []FILRetrievalCandidate
	SelNode    ipld.Node

	// Disable sorting of candidates based on preferability
	NoSort bool
}

func (attempt *FILRetrievalAttempt) Retrieve(ctx context.Context, node *whypfs.Node) (RetrievalStats, error) {
	// If no miners are provided, there's nothing else we can do
	if len(attempt.Candidates) == 0 {
		log.Info("No miners were provided, will not attempt FIL retrieval")
		return nil, xerrors.Errorf("retrieval failed: no miners were provided")
	}

	// If IPFS retrieval was unavailable, do a full FIL retrieval. Start with
	// querying all the candidates for sorting.

	log.Info("Querying FIL retrieval candidates...")

	type CandidateQuery struct {
		Candidate FILRetrievalCandidate
		Response  *retrievalmarket.QueryResponse
	}
	checked := 0
	var queries []CandidateQuery
	var queriesLk sync.Mutex

	var wg sync.WaitGroup
	wg.Add(len(attempt.Candidates))

	for _, candidate := range attempt.Candidates {

		// Copy into loop, cursed go
		candidate := candidate

		go func() {
			defer wg.Done()

			query, err := attempt.FilClient.RetrievalQuery(ctx, candidate.Miner, candidate.RootCid)
			if err != nil {
				log.Debugf("Retrieval query for miner %s failed: %v", candidate.Miner, err)
				return
			}

			queriesLk.Lock()
			queries = append(queries, CandidateQuery{Candidate: candidate, Response: query})
			checked++
			fmt.Fprintf(os.Stderr, "%v/%v\r", checked, len(attempt.Candidates))
			queriesLk.Unlock()
		}()
	}

	wg.Wait()

	log.Infof("Got back %v retrieval query results of a total of %v candidates", len(queries), len(attempt.Candidates))

	if len(queries) == 0 {
		return nil, xerrors.Errorf("retrieval failed: queries failed for all miners")
	}

	// After we got the query results, sort them with respect to the candidate
	// selection config as long as noSort isn't requested (TODO - more options)

	if !attempt.NoSort {
		sort.Slice(queries, func(i, j int) bool {
			a := queries[i].Response
			b := queries[i].Response

			// Always prefer unsealed to sealed, no matter what
			if a.UnsealPrice.IsZero() && !b.UnsealPrice.IsZero() {
				return true
			}

			// Select lower price, or continue if equal
			aTotalPrice := totalCost(a)
			bTotalPrice := totalCost(b)
			if !aTotalPrice.Equals(bTotalPrice) {
				return aTotalPrice.LessThan(bTotalPrice)
			}

			// Select smaller size, or continue if equal
			if a.Size != b.Size {
				return a.Size < b.Size
			}

			return false
		})
	}

	// Now attempt retrievals in serial from first to last, until one works.
	// stats will get set if a retrieval succeeds - if no retrievals work, it
	// will still be nil after the loop finishes
	var stats *FILRetrievalStats = nil
	for _, query := range queries {
		log.Infof("Attempting FIL retrieval with miner %s from root CID %s (%s)", query.Candidate.Miner, query.Candidate.RootCid, types.FIL(totalCost(query.Response)))

		if attempt.SelNode != nil && !attempt.SelNode.IsNull() {
			log.Infof("Using selector %s", attempt.SelNode)
		}

		proposal, err := retrievehelper.RetrievalProposalForAsk(query.Response, query.Candidate.RootCid, attempt.SelNode)
		if err != nil {
			log.Debugf("Failed to create retrieval proposal with candidate miner %s: %v", query.Candidate.Miner, err)
			continue
		}

		var bytesReceived uint64
		stats_, err := attempt.FilClient.RetrieveContentWithProgressCallback(
			ctx,
			query.Candidate.Miner,
			proposal,
			func(bytesReceived_ uint64) {
				bytesReceived = bytesReceived_
				printProgress(bytesReceived)
			},
		)
		if err != nil {
			log.Errorf("Failed to retrieve content with candidate miner %s: %v", query.Candidate.Miner, err)
			continue
		}

		stats = &FILRetrievalStats{RetrievalStats: *stats_}
		break
	}

	if stats == nil {
		return nil, xerrors.New("retrieval failed for all miners")
	}

	log.Info("FIL retrieval succeeded")

	return stats, nil
}

type FILRetrievalCandidate struct {
	Miner   address.Address
	RootCid cid.Cid
	DealID  uint
}

func GetRetrievalCandidates(endpoint string, c cid.Cid) ([]FILRetrievalCandidate, error) {

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, xerrors.Errorf("endpoint %s is not a valid url", endpoint)
	}
	endpointURL.Path = path.Join(endpointURL.Path, c.String())

	resp, err := http.Get(endpointURL.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http request to endpoint %s got status %v", endpointURL, resp.StatusCode)
	}

	var res []FILRetrievalCandidate

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, xerrors.Errorf("could not unmarshal http response for cid %s", c)
	}

	return res, nil
}
