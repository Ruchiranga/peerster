package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dedis/protobuf"
	"net"
	"net/http"
	"strings"
	"sync"
)

type SimpleMessage struct {
	OriginalName string
	RelayPeerAddr string
	Contents string
}

type GossipPacket struct {
	Simple * SimpleMessage
}

type Gossiper struct {
	gossipAddress * net.UDPAddr
	gossipConn * net.UDPConn
	uiPort string
	Name string
	Peers []string
}

func (gossiper *Gossiper) rememberPeer(address string) {
	if address == gossiper.gossipAddress.String() { // Being resilient to other nodes that might have bugs
		return
	}
	for _, str := range gossiper.Peers {
		if str == address {
			return
		}
	}
	gossiper.Peers = append(gossiper.Peers, address)
}


func (gossiper *Gossiper) listenGossip(wg *sync.WaitGroup) {
	defer gossiper.gossipConn.Close()
	defer wg.Done()

	for {
		var packet GossipPacket

		packetBytes := make([]byte, 4096)
		gossiper.gossipConn.ReadFromUDP(packetBytes)
		protobuf.Decode(packetBytes, &packet)
		gossiper.rememberPeer(packet.Simple.RelayPeerAddr)

		fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", packet.Simple.OriginalName, packet.Simple.RelayPeerAddr, packet.Simple.Contents)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		go gossiper.broadcast(packet)
	}
}

func (gossiper *Gossiper) listenUi(wg *sync.WaitGroup) {
	defer wg.Done()

	handler := http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var packet GossipPacket
		err := decoder.Decode(&packet)
		if err != nil {
			panic(err)
		}
		fmt.Printf("CLIENT MESSAGE %s\n", packet.Simple.Contents)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		go gossiper.broadcast(packet)
	})

	err := http.ListenAndServe(fmt.Sprintf(":%s", gossiper.uiPort), handler)
	if err != nil {
		panic(err)
	}
}

func (gossiper *Gossiper) broadcast(packet GossipPacket) {
	if packet.Simple.OriginalName == "" {
		packet.Simple.OriginalName = gossiper.Name
	}

	relayPeerAddress := packet.Simple.RelayPeerAddr
	packet.Simple.RelayPeerAddr = gossiper.gossipAddress.String()
	for _, peerAddress := range gossiper.Peers {
		if peerAddress != relayPeerAddress {
			destinationAddress, _ := net.ResolveUDPAddr("udp", peerAddress)

			packetBytes, err := protobuf.Encode(&packet)
			if err != nil {
				panic(err)
			}

			gossiper.gossipConn.WriteToUDP(packetBytes, destinationAddress)
		}
	}
}

func main() {
	uiPort := flag.String("UIPort", "8080", "Port for the UI client")
	gossipAddress := flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
	name := flag.String("name", "node-ruchiranga", "Name of the gossiper")
	peersString := flag.String("peers", "127.0.0.1:5001,10.1.1.7:5002", "comma separated list of peers of the form ip:port")
	// Change default value depending on the requirement
	simple := flag.Bool("simple", true, "run gossiper in simple broadcast mode")

	flag.Parse()

	peersList := strings.Split(*peersString, ",")

	gossiper := NewGossiper(*gossipAddress, *name, peersList, *uiPort)

	var wg sync.WaitGroup
	wg.Add(2)

	go gossiper.listenUi(&wg)
	go gossiper.listenGossip(&wg)

	wg.Wait()

	fmt.Println(*simple)
}

func NewGossiper(address, name string, peers []string, uiPort string) *Gossiper {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	udpConn, err := net.ListenUDP("udp4", udpAddr)

	if err != nil {
		panic(err)
	}

	return &Gossiper{
		gossipAddress: udpAddr,
		gossipConn: udpConn,
		Name: name,
		Peers: peers,
		uiPort: uiPort,
	}
}