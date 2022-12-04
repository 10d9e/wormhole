package filecoin

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/mitchellh/go-homedir"

	"github.com/anacrolix/log"
	"github.com/application-research/filclient"
	"github.com/application-research/filclient/keystore"
	whypfs "github.com/application-research/whypfs-core"
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	lcli "github.com/filecoin-project/lotus/cli"
)

const (
	NetworkFIL  = "fil"
	NetworkIPFS = "ipfs"
	NetworkAuto = "auto"
)

/*
type Node struct {
	Host host.Host

	Datastore  datastore.Batching
	DHT        *dht.IpfsDHT
	Blockstore blockstore.Blockstore
	Bitswap    *bitswap.Bitswap

	Wallet *wallet.LocalWallet
}
*/

//var wal *wallet.LocalWallet

var ApiURL string = "wss://api.chain.love"

func Get(nd *whypfs.Node, cidStr, network, dmSelText string, minerStrings []string) error {

	homedir, err := ddir()
	if err != nil {
		return err
	}

	wal, err := setup(homedir)
	if err != nil {
		return err
	}
	fmt.Println(wal)

	// Parse command input
	if cidStr == "" {
		return fmt.Errorf("please specify a CID to retrieve")
	}

	// dmSelText := textselector.Expression(cctx.String(flagDmPathSel.Name))

	miners, err := parseMiners(minerStrings)
	if err != nil {
		return err
	}

	c, err := cid.Decode(cidStr)
	if err != nil {
		return err
	}

	// Get subselector node
	var selNode ipld.Node
	/*
		var selNode ipld.Node
		if dmSelText != "" {
			ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)

			selspec, err := textselector.SelectorSpecFromPath(
				dmSelText,
				true,

				// URGH - this is a direct copy from https://github.com/filecoin-project/go-fil-markets/blob/v1.12.0/shared/selectors.go#L10-L16
				// Unable to use it because we need the SelectorSpec, and markets exposes just a reified node
				ssb.ExploreRecursive(
					selector.RecursionLimitNone(),
					ssb.ExploreAll(ssb.ExploreRecursiveEdge()),
				),
			)
			if err != nil {
				return xerrors.Errorf("failed to parse text-selector '%s': %w", dmSelText, err)
			}

			selNode = selspec.Node()
		}
	*/

	// Set up node and filclient

	ddir, err := ddir()
	if err != nil {
		return err
	}

	/*
		wallet, err := setup(ddir)
		if err != nil {
			return err
		}
	*/

	fc, closer, err := clientFromNode(nd, wal, ddir)
	if err != nil {
		return err
	}
	defer closer()

	// Collect retrieval candidates and config. If one or more miners are
	// provided, use those with the requested cid as the root cid as the
	// candidate list. Otherwise, we can use the auto retrieve API endpoint
	// to automatically find some candidates to retrieve from.

	var candidates []FILRetrievalCandidate
	if len(miners) > 0 {
		for _, miner := range miners {
			candidates = append(candidates, FILRetrievalCandidate{
				Miner:   miner,
				RootCid: c,
			})
		}
	}
	/*
		else {
			endpoint := "https://api.estuary.tech/retrieval-candidates" // TODO: don't hard code
			candidates_, err := node.GetRetrievalCandidates(endpoint, c)
			if err != nil {
				return fmt.Errorf("failed to get retrieval candidates: %w", err)
			}

			candidates = candidates_
		}
	*/

	// Do the retrieval

	var networks []GetAttempt

	if network == NetworkIPFS || network == NetworkAuto {
		/*
			if selNode != nil && !selNode.IsNull() {
				// Selector nodes are not compatible with IPFS
				if network == NetworkIPFS {
					log.Fatal("IPFS is not compatible with selector node")
				} else {
					log.Info("A selector node has been specified, skipping IPFS")
				}
			} else {
				networks = append(networks, &IPFSRetrievalAttempt{
					Cid: c,
				})
			}
		*/

		networks = append(networks, &IPFSRetrievalAttempt{
			Cid: c,
		})
	}

	if network == NetworkFIL || network == NetworkAuto {
		networks = append(networks, &FILRetrievalAttempt{
			FilClient:  fc,
			Cid:        c,
			Candidates: candidates,
			SelNode:    selNode,
		})
	}

	if len(networks) == 0 {
		log.Fatalf("Unknown --network value \"%s\"", network)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stats, err := RetrieveFromBestCandidate(nd, ctx, networks)
	if err != nil {
		return err
	}

	printRetrievalStats(stats)

	// Save the output

	// jcl
	//dservOffline := merkledag.NewDAGService(blockservice.New(node.Blockstore, offline.Exchange(node.Blockstore)))

	// if we used a selector - need to find the sub-root the user actually wanted to retrieve
	/*
		if dmSelText != "" {
			var subRootFound bool

			// no err check - we just compiled this before starting, but now we do not wrap a `*`
			selspec, _ := textselector.SelectorSpecFromPath(dmSelText, true, nil) //nolint:errcheck
			if err := retrievehelper.TraverseDag(
				cctx.Context,
				dservOffline,
				c,
				selspec.Node(),
				func(p traversal.Progress, n ipld.Node, r traversal.VisitReason) error {
					if r == traversal.VisitReason_SelectionMatch {

						if p.LastBlock.Path.String() != p.Path.String() {
							return xerrors.Errorf("unsupported selection path '%s' does not correspond to a node boundary (a.k.a. CID link)", p.Path.String())
						}

						cidLnk, castOK := p.LastBlock.Link.(cidlink.Link)
						if !castOK {
							return xerrors.Errorf("cidlink cast unexpectedly failed on '%s'", p.LastBlock.Link.String())
						}

						c = cidLnk.Cid
						subRootFound = true
					}
					return nil
				},
			); err != nil {
				return xerrors.Errorf("error while locating partial retrieval sub-root: %w", err)
			}

			if !subRootFound {
				return xerrors.Errorf("path selection '%s' does not match a node within %s", dmSelText, c)
			}
		}
	*/

	// jcl
	/*
		dnode, err := dservOffline.Get(ctx, c)
		if err != nil {
			return err
		}
	*/

	/*
		if cctx.Bool(flagCar.Name) {
			// Write file as car file
			file, err := os.Create(output + ".car")
			if err != nil {
				return err
			}
			car.WriteCar(cctx.Context, dservOffline, []cid.Cid{c}, file)

			fmt.Println("Saved .car output to", output+".car")
		} else {
			// Otherwise write file as UnixFS File
			ufsFile, err := unixfile.NewUnixfsFile(cctx.Context, dservOffline, dnode)
			if err != nil {
				return err
			}

			if err := files.WriteTo(ufsFile, output); err != nil {
				return err
			}

			fmt.Println("Saved output to", output)
		}
	*/

	// jcl
	// Otherwise write file as UnixFS File
	/*
		ufsFile, err := unixfile.NewUnixfsFile(ctx, dservOffline, dnode)
		if err != nil {
			return err
		}

		//jcl
		output := cidStr

		if err := files.WriteTo(ufsFile, output); err != nil {
			return err
		}

		fmt.Println("Saved output to", output)
	*/

	return nil
}

func walletPath(baseDir string) string {
	return filepath.Join(baseDir, "wallet")
}

func setup(cfgdir string) (*wallet.LocalWallet, error) {
	return setupWallet(walletPath(cfgdir))
}

// Read a comma-separated or multi flag list of miners from the CLI.
func parseMiners(minerStrings []string) ([]address.Address, error) {
	var miners []address.Address
	for _, ms := range minerStrings {

		miner, err := address.NewFromString(ms)
		if err != nil {
			return nil, fmt.Errorf("failed to parse miner %s: %w", ms, err)
		}

		miners = append(miners, miner)
	}

	return miners, nil
}

func clientFromNode(nd *whypfs.Node, wal *wallet.LocalWallet, dir string) (*filclient.FilClient, func(), error) {
	// send a CLI context to lotus that contains only the node "api-url" flag set, so that other flags don't accidentally conflict with lotus cli flags
	// https://github.com/filecoin-project/lotus/blob/731da455d46cb88ee5de9a70920a2d29dec9365c/cli/util/api.go#L37
	flset := flag.NewFlagSet("lotus", flag.ExitOnError)
	flset.String("api-url", "", "node api url")
	err := flset.Set("api-url", ApiURL)
	if err != nil {
		return nil, nil, err
	}

	ncctx := cli.NewContext(cli.NewApp(), flset, nil)
	api, closer, err := lcli.GetGatewayAPI(ncctx)
	if err != nil {
		return nil, nil, err
	}
	//defer closer()

	addr, err := wal.GetDefault()
	if err != nil {
		return nil, nil, err
	}

	fc, err := filclient.NewClient(nd.Host, api, wal, addr, nd.Blockstore, nd.Datastore, dir)
	if err != nil {
		return nil, nil, err
	}

	return fc, closer, nil
}

func setupWallet(dir string) (*wallet.LocalWallet, error) {
	kstore, err := keystore.OpenOrInitKeystore(dir)
	if err != nil {
		return nil, err
	}

	wallet, err := wallet.NewWallet(kstore)
	if err != nil {
		return nil, err
	}

	addrs, err := wallet.WalletList(context.TODO())
	if err != nil {
		return nil, err
	}

	if len(addrs) == 0 {
		_, err := wallet.WalletNew(context.TODO(), types.KTBLS)
		if err != nil {
			return nil, err
		}
	}

	return wallet, nil
}

func ddir() (string, error) {
	// Store config dir in metadata
	ddir, err := homedir.Expand("~/.whypfs")
	if err != nil {
		fmt.Println("could not set config dir: ", err)
	}
	return ddir, err
}
