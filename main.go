package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	whypfs "github.com/application-research/whypfs-core"
	"github.com/ipfs/go-cid"
	leveldb "github.com/ipfs/go-ds-leveldb"
	fc "github.com/jlogelin/wormhole/filecoin"
)

var (
	// OsSignal signal used to shutdown
	OsSignal chan os.Signal
)

func main() {
	OsSignal = make(chan os.Signal, 1)
	BootstrapWhyPFS()
	LoopForever()
}

// LoopForever on signal processing
func LoopForever() {
	fmt.Printf("Entering infinite loop\n")

	signal.Notify(OsSignal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	<-OsSignal

	fmt.Printf("Exiting infinite loop received OsSignal\n")
}

func BootstrapWhyPFS() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	/*
		node, err := whypfs.NewNode(whypfs.NewNodeParams{
			Ctx: ctx,
			// Datastore: whypfs.NewInMemoryDatastore(),
			Config: optimalConfig(),
		})
	*/

	node, err := whypfs.NewNode(
		whypfs.NewNodeParams{
			Ctx: context.Background(),
			Config: &whypfs.Config{
				Libp2pKeyFile: "libp2p.key",
				ListenAddrs:   []string{"/ip4/0.0.0.0/tcp/6746"},
				AnnounceAddrs: nil,
				DatastoreDir: struct {
					Directory string
					Options   leveldb.Options
				}{
					Directory: "datastore",
					Options:   leveldb.Options{},
				},
				Blockstore:        ":flatfs:.whypfs/blocks",
				NoBlockstoreCache: false,
				NoLimiter:         true,
				BitswapConfig: whypfs.BitswapConfig{
					MaxOutstandingBytesPerPeer: 20 << 20,
					TargetMessageSize:          2 << 20,
				},
				//LimitsConfig            Limits
				ConnectionManagerConfig: whypfs.ConnectionManager{},
			},
		})
	check(err)

	node.BootstrapPeers(whypfs.DefaultBootstrapPeers())
	check(err)

	fmt.Printf("Using peer ID: %s \n", node.Host.ID())
	fmt.Printf("Addrs: %s \n", node.Host.Addrs())

	/*
		c1, _ := cid.Decode("QmbJGGJkjGfYCmHqwoMjLTbUA6bdcFBbNdWChFY6dKNRWx")
		rsc1, err := node.GetFile(ctx, c1)
		if err != nil {
			panic(err)
		}
		defer rsc1.Close()
		content1, err := io.ReadAll(rsc1)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(content1))

	*/

	file, err := node.AddPinFile(context.Background(), bytes.NewReader([]byte("lodgewasheresdfsdfsd!")), nil)
	if err != nil {
		return
	}

	test, err := node.GetFile(ctx, file.Cid())
	check(err)
	defer test.Close()
	cont1, err := io.ReadAll(test)
	check(err)
	fmt.Println("File CID: ", file.Cid().String())
	ff, err := os.Create(file.Cid().String())
	check(err)
	nn2, err := ff.Write(cont1)
	check(err)
	fmt.Printf("wrote %d bytes\n", nn2)

	cidStr := "bafybeibyzkigxj527gw7iq5jz6mkd2xgcxhny3ukmvrkrn3vvqpiqiuohe"
	// cidStr := "bafkreif25pcqu5wo24bf3bxeybbq2snehcqd5nx2ar5b2eez6gey2lwt5q"

	network := fc.NetworkAuto
	// network := NetworkIPFS
	err = fc.Get(node, cidStr, network, "", []string{"f010088"})
	check(err)

	c1, _ := cid.Decode(cidStr)
	rsc1, err := node.GetFile(ctx, c1)
	check(err)
	defer rsc1.Close()
	content1, err := io.ReadAll(rsc1)
	check(err)

	f, err := os.Create(cidStr)
	check(err)
	n2, err := f.Write(content1)
	check(err)
	fmt.Printf("wrote %d bytes\n", n2)

	//fmt.Println(string(content1))
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}
