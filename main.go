package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	whypfs "github.com/application-research/whypfs-core"
	"github.com/ipfs/go-cid"
	levelds "github.com/ipfs/go-ds-leveldb"
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

func optimalConfig() *whypfs.Config {
	// optimal settings
	cfg := &whypfs.Config{}
	cfg.Offline = false
	cfg.ReprovideInterval = 8 * time.Hour
	cfg.NoBlockstoreCache = false
	cfg.NoAnnounceContent = false
	cfg.NoLimiter = false
	cfg.BitswapConfig.MaxOutstandingBytesPerPeer = 20 << 20
	cfg.BitswapConfig.TargetMessageSize = 2 << 20
	cfg.ConnectionManagerConfig.HighWater = 1000
	cfg.ConnectionManagerConfig.LowWater = 900
	cfg.DatastoreDir.Directory = "datastore"
	cfg.DatastoreDir.Options = levelds.Options{}
	cfg.Blockstore = ":flatfs:.whypfs/blocks"
	cfg.Libp2pKeyFile = filepath.Join("libp2p.key")
	cfg.ListenAddrs = []string{"/ip4/0.0.0.0/tcp/6746"}
	cfg.AnnounceAddrs = []string{"/ip4/0.0.0.0/tcp/0"}
	return cfg
}

func BootstrapWhyPFS() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node, err := whypfs.NewNode(whypfs.NewNodeParams{
		Ctx:       ctx,
		Datastore: whypfs.NewInMemoryDatastore(),
		Config:    optimalConfig(),
	})

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

	file, err := node.AddPinFile(context.Background(), bytes.NewReader([]byte("lodgewashere!")), nil)
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
