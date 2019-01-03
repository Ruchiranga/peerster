package main

import (
	"crypto/rsa"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

var NodeIDLength int
var Verbose bool

func main() {
	Verbose = true
	NodeIDLength = 8

	DEBUG := false

	uiPort := flag.String("UIPort", "8080", "Port for the UI client")
	gossipAddress := flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
	name := flag.String("name", "", "Name of the gossiper")
	peersString := flag.String("peers", "", "comma separated list of peers of the form ip:port")
	simple := flag.Bool("simple", false, "run gossiper in simple broadcast mode")
	rtimer := flag.Int("rtimer", 0, "route rumors sending period in seconds, 0 to disable sending of route rumors")

	flag.Parse()

	var peersList []string
	if *peersString == "" {
		peersList = []string{}
	} else {
		peersList = strings.Split(*peersString, ",")
		uniquePeersList := []string{}
	outer:
		for _, peer := range peersList {
			for _, uniquePeer := range uniquePeersList {
				if peer == uniquePeer {
					continue outer
				}
			}
			uniquePeersList = append(uniquePeersList, peer)
		}
		peersList = uniquePeersList
	}

	fmt.Println("Peers", peersList)
	gossiper := NewGossiper(*name, *gossipAddress, peersList, *uiPort, *simple, *rtimer, DEBUG)

	var wg sync.WaitGroup
	wg.Add(6)

	gossiper.initKey(gossiper.Name + "-" + DefaultKeyFileName)
	go gossiper.initializePastry()
	go gossiper.executeJobs(&wg)
	go gossiper.executeBlockChainJobs(&wg)
	go gossiper.listenUi(&wg)
	go gossiper.listenGossip(&wg)
	go gossiper.doAntiEntropy(&wg)

	go gossiper.bootstrapBlockchain()

	go gossiper.txChannelListener(&wg)
	if *rtimer > 0 {
		wg.Add(1)
		go gossiper.announceRoutes(&wg)
	}

	go gossiper.txPublishMyKey(gossiper.key, gossiper.Name)

	wg.Wait()
}

func NewGossiper(name string, address string, peers []string, uiPort string, simple bool, rtimer int, debug bool) *Gossiper {
	udpAddr, addrErr := net.ResolveUDPAddr("udp4", address)
	if addrErr != nil {
		panic(addrErr)
	}
	udpConn, connErr := net.ListenUDP("udp4", udpAddr)
	if connErr != nil {
		panic(connErr)
	}

	if name == "" {
		name = generateResourceId()
	}
	fmt.Printf("My name is %s\n", name)
	messagesMap := make(map[string][]GenericMessage)
	ackAwaitMap := make(map[string]func(status StatusPacket))
	routingTable := make(map[string]string)
	fileContentMap := make(map[string][]byte)
	fileAwaitMap := make(map[string]func(reply DataReply))
	searchAwaitMap := make(map[string]func(reply SearchReply))
	currentDownloads := make(map[string][]byte)
	lastSearchRequest := make(map[string]int64)
	searchResults := make(map[string]map[uint64][]string)
	fileMetaMap := make(map[string][]byte)
	fileReplicateAwaitMap := make(map[string]func(ack FileReplicateAck))
	fileReplicatedTargetsMap := make(map[string][]string)
	fileStreamableSrcMap := make(map[string]string)
	keyMap := make(map[string]*rsa.PublicKey)
	// Jobs channel length did not seem to exceed 10 items even at high loads.
	// Hence a value of 20 is given keeping a buffer.
	jobsChannel := make(chan func(), 20)
	blockChainEventLoop := make(chan func(), 100)
	txChannel := make(chan TxPublish, 1000)
	var forks [][]Block
	var strayBlocks []Block

	randSource := rand.NewSource(time.Now().UTC().UnixNano())
	randGen := rand.New(randSource)

	entropyTicker := time.NewTicker(time.Second)
	var routingTicker *time.Ticker
	if rtimer > 0 {
		routingTicker = time.NewTicker(time.Duration(rtimer) * time.Second)
	}

	neighbours := make([]Peer, 0, 8)
	var pastryRoutingTable [8][4]Peer
	upperLeafSet := make([]Peer, 0, 4)
	lowerLeafSet := make([]Peer, 0, 4)

	return &Gossiper{
		jobsChannel:              jobsChannel,
		gossipAddress:            udpAddr,
		gossipConn:               udpConn,
		Name:                     name,
		Peers:                    peers,
		uiPort:                   uiPort,
		simple:                   simple,
		ackAwaitMap:              ackAwaitMap,
		messagesMap:              messagesMap,
		routingTable:             routingTable,
		randGen:                  randGen,
		debug:                    debug,
		entropyTicker:            entropyTicker,
		routingTicker:            routingTicker,
		fileContentMap:           fileContentMap,
		fileAwaitMap:             fileAwaitMap,
		searchAwaitMap:           searchAwaitMap,
		currentDownloads:         currentDownloads,
		lastSearchRequest:        lastSearchRequest,
		searchResults:            searchResults,
		fileMetaMap:              fileMetaMap,
		forks:                    forks,
		blockChainEventLoop:      blockChainEventLoop,
		txChannel:                txChannel,
		strayBlocks:              strayBlocks,
		neighbours:               neighbours,
		pastryRoutingTable:       pastryRoutingTable,
		upperLeafSet:             upperLeafSet,
		lowerLeafSet:             lowerLeafSet,
		fileReplicateAwaitMap:    fileReplicateAwaitMap,
		fileReplicatedTargetsMap: fileReplicatedTargetsMap,
		fileStreamableSrcMap:     fileStreamableSrcMap,
		keyMap:                   keyMap,
		blockchainBootstrap:      false,
	}
}
