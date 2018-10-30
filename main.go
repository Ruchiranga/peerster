package main

import (
	"flag"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

func main() {
	DEBUG := true

	uiPort := flag.String("UIPort", "8080", "Port for the UI client")
	gossipAddress := flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
	name := flag.String("name", "node-ruchiranga", "Name of the gossiper")
	peersString := flag.String("peers", "127.0.0.1:5001,10.1.1.7:5002", "comma separated list of peers of the form ip:port")
	simple := flag.Bool("simple", false, "run gossiper in simple broadcast mode")

	flag.Parse()

	var peersList []string
	if *peersString == "" {
		peersList = []string{}
	} else {
		peersList = strings.Split(*peersString, ",")
	}

	gossiper := NewGossiper(*gossipAddress, *name, peersList, *uiPort, *simple, DEBUG)

	var wg sync.WaitGroup
	wg.Add(4)

	go gossiper.listenUi(&wg)
	go gossiper.listenGossip(&wg)
	go gossiper.doAntiEntropy(&wg)
	go gossiper.executeJobs(&wg)

	wg.Wait()
}

func NewGossiper(address, name string, peers []string, uiPort string, simple bool, debug bool) *Gossiper {
	udpAddr, addrErr := net.ResolveUDPAddr("udp4", address)
	if addrErr != nil {
		panic(addrErr)
	}
	udpConn, connErr := net.ListenUDP("udp4", udpAddr)
	if connErr != nil {
		panic(connErr)
	}

	messagesMap := make(map[string][]RumorMessage)
	ackAwaitMap := make(map[string] func(status StatusPacket))

	// Jobs channel length did not seem to exceed 10 items even at high loads.
	// Hence a value of 20 is given keeping a buffer.
	jobsChannel := make(chan func(), 20)

	randSource := rand.NewSource(time.Now().UTC().UnixNano())
	randGen := rand.New(randSource)

	ticker := time.NewTicker(time.Second)

	return &Gossiper{
		jobsChannel:   jobsChannel,
		gossipAddress: udpAddr,
		gossipConn:    udpConn,
		Name:          name,
		Peers:         peers,
		uiPort:        uiPort,
		simple:        simple,
		ackAwaitMap:   ackAwaitMap,
		messagesMap:   messagesMap,
		randGen:       randGen,
		debug:         debug,
		ticker:        ticker,
	}
}
