package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dedis/protobuf"
	"net"
	"net/http"
	"strings"
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
	gossipAddressStr string
	gossipAddress * net.UDPAddr
	gossipConn * net.UDPConn
	uiPort string
	Name string
	Peers []string
	sendChan chan GossipPacket
	receiveChan chan GossipPacket
}

func (gossiper *Gossiper) rememberPeer(address string) {
	if address == gossiper.gossipAddress.String() {
		return
	}
	for _, str := range gossiper.Peers {
		if str == address {
			return
		}
	}
	gossiper.Peers = append(gossiper.Peers, address)
}


func (gossiper *Gossiper) listenGossip() {
	defer gossiper.gossipConn.Close()
	var packet GossipPacket
	for {
		packetBytes := make([]byte, 4096)
		gossiper.gossipConn.ReadFromUDP(packetBytes)
		protobuf.Decode(packetBytes, &packet)
		gossiper.rememberPeer(packet.Simple.RelayPeerAddr)

		fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", packet.Simple.OriginalName, packet.Simple.RelayPeerAddr, packet.Simple.Contents)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		gossiper.receiveChan <- packet
	}
}

func (gossiper *Gossiper) listenUi() {
	handler := http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var packet GossipPacket
		err := decoder.Decode(&packet)
		if err != nil {
			panic(err)
		}
		fmt.Printf("CLIENT MESSAGE %s\n", packet.Simple.Contents)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		gossiper.receiveChan <- packet
	})

	err := http.ListenAndServe(fmt.Sprintf(":%s", gossiper.uiPort), handler)
	if err != nil {
		panic(err)
	}
}

//func (gossiper *Gossiper) broadcastOnce(packet GossipPacket) {
//
//	relayPeerAddress := packet.Simple.RelayPeerAddr
//	packet.Simple.RelayPeerAddr = gossiper.gossipAddressStr
//	for _, peerAddress := range gossiper.Peers {
//		if peerAddress != relayPeerAddress {
//			destinationAddress, _ := net.ResolveUDPAddr("udp", peerAddress)
//
//			packetBytes, err := protobuf.Encode(&packet)
//			if err != nil {
//				panic(err)
//			}
//			if packet.Simple.RelayPeerAddr == gossiper.gossipAddressStr {
//				fmt.Println(packet.Simple.RelayPeerAddr == gossiper.gossipAddressStr, packet.Simple.RelayPeerAddr, gossiper.gossipAddressStr)
//				gossiper.gossipConn.WriteToUDP(packetBytes, destinationAddress)
//			}
//		}
//	}
//
//}

func (gossiper *Gossiper) broadcast() {
	for {
		packet := <- gossiper.sendChan
		relayPeerAddress := packet.Simple.RelayPeerAddr
		packet.Simple.RelayPeerAddr = gossiper.gossipAddressStr
		for _, peerAddress := range gossiper.Peers {
			if peerAddress != relayPeerAddress {
				destinationAddress, _ := net.ResolveUDPAddr("udp", peerAddress)

				packetBytes, err := protobuf.Encode(&packet)
				if err != nil {
					panic(err)
				}
				if packet.Simple.RelayPeerAddr == gossiper.gossipAddressStr {
					fmt.Println(packet.Simple.RelayPeerAddr == gossiper.gossipAddressStr, packet.Simple.RelayPeerAddr, gossiper.gossipAddressStr)

					gossiper.gossipConn.WriteToUDP(packetBytes, destinationAddress)

				}
			}
		}
	}
}

func main() {
	uiPort := flag.String(
		"UIPort", "8080", "Port for the UI client")
	gossipAddress := flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
	name := flag.String("name", "node-ruchiranga", "Name of the gossiper")
	peersString := flag.String("peers", "127.0.0.1:5001,10.1.1.7:5002", "comma separated list of peers of the form ip:port")
	// Change default value depending on the requirement
	simple := flag.Bool("simple", true, "run gossiper in simple broadcast mode")

	flag.Parse()

	peersList := strings.Split(*peersString, ",")

	gossiper := NewGossiper(*gossipAddress, *name, peersList, *uiPort)

	go gossiper.listenUi()
	go gossiper.listenGossip()
	go gossiper.broadcast()

	for {
		packet := <-gossiper.receiveChan

		if packet.Simple.RelayPeerAddr == "" && packet.Simple.OriginalName == "" {
			packet.Simple.OriginalName = *name
		}

		gossiper.sendChan <- packet
	}

	fmt.Println(*simple)
}

func NewGossiper(address, name string, peers []string, uiPort string) *Gossiper {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	udpConn, err := net.ListenUDP("udp4", udpAddr)

	if err != nil {
		panic(err)
	}

	sendChan := make(chan GossipPacket, 5)
	receiveChan := make(chan GossipPacket, 5)

	return &Gossiper{
		gossipAddressStr: address,
		gossipAddress: udpAddr,
		gossipConn: udpConn,
		Name: name,
		Peers: peers,
		uiPort: uiPort,
		sendChan: sendChan,
		receiveChan: receiveChan,
	}
}