package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dedis/protobuf"
	"log"
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
	gossipAddressStr string
	gossipConn * net.PacketConn
	uiListener * net.Listener
	uiPort string
	Name string
	Peers []string
}

func (gossiper *Gossiper) rememberPeer(address string) {
	//if address == gossiper.gossipAddress.String() {
	//	return
	//}
	for _, str := range gossiper.Peers {
		if str == address {
			return
		}
	}
	gossiper.Peers = append(gossiper.Peers, address)
}

func (gossiper *Gossiper) listenGossip(wg * sync.WaitGroup) {

	defer (*gossiper.gossipConn).Close()
	defer wg.Done()

	for {
		var packet GossipPacket
		buf := make([]byte, 4096)
		_, _, err := (*gossiper.gossipConn).ReadFrom(buf)
		//n, addr, err := (*gossiper.gossipConn).ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
			fmt.Println("Continuing to receive UDP despite of error...")
			continue
		}
		protobuf.Decode(buf, &packet)
		gossiper.rememberPeer(packet.Simple.RelayPeerAddr)

		fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", packet.Simple.OriginalName, packet.Simple.RelayPeerAddr, packet.Simple.Contents)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		go gossiper.broadcast(packet)
	}
}

func (gossiper *Gossiper) listenUi(wg * sync.WaitGroup) {
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
	senderAddress := packet.Simple.RelayPeerAddr
	packet.Simple.RelayPeerAddr = gossiper.gossipAddressStr
	for _, address := range gossiper.Peers {
		if address != senderAddress {
			dst, err := net.ResolveUDPAddr("udp", address)
			if err != nil {
				panic(err)
			}

			buf, err := protobuf.Encode(&packet)
			_, err = (*gossiper.gossipConn).WriteTo(buf, dst)
			if err != nil {
				panic(err)
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

	var wg sync.WaitGroup
	wg.Add(2)

	go gossiper.listenUi(&wg)
	go gossiper.listenGossip(&wg)

	wg.Wait()
	fmt.Println(*simple)
}

func NewGossiper(address, name string, peers []string, uiPort string) *Gossiper {
	port := strings.Split(address, ":")[1]
	udpConn, errUdp := net.ListenPacket("udp", fmt.Sprintf(":%s", port))

	tcpL, errTcp := net.Listen("tcp", fmt.Sprintf("localhost:%s", uiPort)) // localhost or gossiper IP?

	if errUdp != nil {
		panic(errUdp)
	}
	if errTcp != nil {
		panic(errTcp)
	}

	return &Gossiper{
		gossipAddressStr: address,
		gossipConn: &udpConn,
		uiListener: &tcpL,
		Name: name,
		Peers: peers,
		uiPort: uiPort,
	}
}