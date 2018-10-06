package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dedis/protobuf"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type SimpleMessage struct {
	OriginalName string
	RelayPeerAddr string
	Contents string
}

type RumourMessage struct {
	Origin string
	ID uint32
	Text string
}

type PeerStatus struct {
	Identifier string
	NextId uint32
}

type StatusPacket struct {
	Want []PeerStatus
}

type GossipPacket struct {
	Simple * SimpleMessage
	Rumour * RumourMessage
	Status * StatusPacket
}

type Gossiper struct {
	gossipAddress * net.UDPAddr
	gossipConn * net.UDPConn
	uiPort string
	Name string
	simple bool
	Peers []string
	ackAwaitList AckAwaitList
	messagesMap MessagesMap
}

type MessagesMap struct {
	sync.RWMutex
	messages map[string][]RumourMessage
}

type AckAwaitList struct {
	sync.RWMutex
	ackChans map[string]chan StatusPacket
}

func (gossiper * Gossiper) rememberPeer(address string) {
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

func (gossiper * Gossiper) listenGossip(wg *sync.WaitGroup) {
	defer gossiper.gossipConn.Close()
	defer wg.Done()

	for {
		var packet GossipPacket

		packetBytes := make([]byte, 4096)
		_, relayPeer, err := gossiper.gossipConn.ReadFromUDP(packetBytes)

		if err != nil {
			log.Fatal(err)
			continue
		}

		protobuf.Decode(packetBytes, &packet)
		gossiper.rememberPeer(relayPeer.String())

		fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", packet.Simple.OriginalName, packet.Simple.RelayPeerAddr, packet.Simple.Contents)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		if gossiper.simple {
			go gossiper.broadcast(packet)
		} else {
			go gossiper.handleGossip(packet, relayPeer.String())
		}


	}
}

func (gossiper * Gossiper) handleGossip(packet GossipPacket, relayPeer string) {
	if packet.Rumour != nil {
		rumour := packet.Rumour

		if gossiper.isRumourNews(rumour) {
			gossiper.storeMessage(rumour)
			statusPacket := gossiper.generateStatusPacket()
			gossiper.broadcastToAddr(GossipPacket{Status: &statusPacket}, relayPeer)
			go gossiper.rumourMonger(packet)
		}
	} else if packet.Status != nil {
		gossiper.ackAwaitList.RLock()
		if gossiper.ackAwaitList.ackChans[relayPeer] != nil {
			gossiper.ackAwaitList.ackChans[relayPeer] <- *packet.Status
		} else {
			// anti-entropy case
			// check vectors and send status or news I have
		}
	} else {
		log.Fatal("Unexpected gossipe packet type.")
	}
}

func (gossiper * Gossiper) rumourMonger(gossip GossipPacket) {

}

func (gossiper *Gossiper) generateStatusPacket() (statusPacket StatusPacket) {
	gossiper.messagesMap.RLock()
	messageMap := gossiper.messagesMap.messages
	gossiper.messagesMap.RUnlock()

	wantList := []PeerStatus{}
	for peer, messages := range messageMap {
		nextId := uint32(len(messages) + 1)
		var idx uint32
		for idx = range messages {
			if messages[idx].ID != idx + 1 {
				nextId = idx + 1
				break
			}
		}
		wantList = append(wantList, PeerStatus{Identifier: peer, NextId: nextId})
	}

	statusPacket = StatusPacket{Want: wantList}
	return
}

func (gossiper * Gossiper) sendStatus(status StatusPacket, address string) {

}

func (gossiper * Gossiper) storeMessage(rumour * RumourMessage) {
	gossiper.messagesMap.RLock()
	messageList := gossiper.messagesMap.messages[rumour.Origin]
	gossiper.messagesMap.RUnlock()

	messageList = append(messageList, *rumour)
	sort.Slice(messageList, func(i, j int) bool {
		return messageList[i].ID < messageList[j].ID
	})

	gossiper.messagesMap.Lock()
	gossiper.messagesMap.messages[rumour.Origin] = messageList
	gossiper.messagesMap.Unlock()
}

func (gossiper * Gossiper) isRumourNews(rumour * RumourMessage) (news bool){
	news = true
	gossiper.messagesMap.RLock()
	for _, message := range gossiper.messagesMap.messages[rumour.Origin] {
		if message.ID == rumour.ID {
			news = false
			break
		}
	}
	gossiper.messagesMap.RUnlock()
	return
}

func rumourMonger(rumour RumourMessage) {

}

func (gossiper * Gossiper) addPeer (address string) {
	// append a nil item to the end of the ackchans aswell

}

func (gossiper * Gossiper) listenUi(wg *sync.WaitGroup) {
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

		// Assuming client is not able to send Status messages
		if packet.Rumour != nil {
			go gossiper.handleRumour(packet, "")
		} else { // SimpleMessage
			go gossiper.broadcast(packet)
		}
	})

	err := http.ListenAndServe(fmt.Sprintf(":%s", gossiper.uiPort), handler)
	if err != nil {
		panic(err)
	}
}
//
//func (gossiper * Gossiper) handleRumour(packet GossipPacket, relayPeer string) {
//
//	// Acknowledge and send them back status if relayPeer != "", i.e not a client rumour
//	if relayPeer != "" && packet.Rumour.Origin != "" {
//		gossiper.updateWantList(*packet.Rumour)
//		status := StatusPacket{ Want : gossiper.wantList}
//		gossipPacket := GossipPacket{Status: &status}
//
//		go gossiper.broadcastToAddr(gossipPacket, relayPeer)
//	} else {
//		packet.Rumour.Origin = gossiper.Name
//		//TODO: set the correct seq number to the rumour ****************************
//	}
//
//	// Send packet to a random peer
//	for {
//		peerIdx := rand.Intn(len(gossiper.Peers))
//
//		peerAckChan := make(chan bool, 5)
//		// Assuming peers and ackChans arrays are parallel
//		chanIdx := -1
//		for idx, peer := range gossiper.Peers {
//			if peer == relayPeer {
//				chanIdx = idx
//				peerAckChan = gossiper.ackChans[idx]
//				break
//			}
//		}
//
//		go gossiper.broadcastToAddr(packet, gossiper.Peers[peerIdx])
//
//		ticker := time.NewTicker(time.Second)
//
//		select {
//			case <- peerAckChan:
//			case <- ticker.C:
//		}
//
//		gossiper.ackChans[chanIdx] = nil
//
//		ticker.Stop()
//
//		i := rand.Int() % 2
//
//		if i == 0 {
//			break
//		} else {
//			continue
//		}
//	}
//}

func (gossiper * Gossiper) updateWantList(rumour RumourMessage) (news bool){
	for idx, peerStatus := range gossiper.wantList {
		if peerStatus.Identifier == rumour.Origin {
			if peerStatus.NextId == rumour.ID {
				gossiper.wantList[idx].NextId += 1
				return true
			}
			// If the rumour ID is ahead of what we want, ok to discard it?
			return false
		}
	}
	newPeerStatus := PeerStatus{ Identifier: rumour.Origin, NextId: 1}
	gossiper.wantList = append(gossiper.wantList, newPeerStatus)
	return true
}

//func (gossiper * Gossiper) handleStatus(packet GossipPacket, relayPeer string) {
//	for idx, peer := range gossiper.Peers {
//		if peer == relayPeer && gossiper.ackChans[idx] != nil {
//			gossiper.ackChans[idx] <- true
//			break
//		}
//	}
//
//	peerWant := packet.Status.Want
//	messageToSend := gossiper.getNextMsgToSend(peerWant)
//
//	packetToSend := GossipPacket{Rumour: &messageToSend}
//	go gossiper.broadcastToAddr(packetToSend, relayPeer)
//
//}

func (gossiper * Gossiper) getNextMsgToSend(peerWants []PeerStatus) (message RumourMessage) {
	for _, peerStatus := range peerWants {
		wantNext := peerStatus.NextId
		wantOrigin := peerStatus.Identifier

		// TODO: avoid this sort by adding messages to the array in the right order
		gossiper.messagesMap.Lock()
		sort.Slice(gossiper.messagesMap.messages[wantOrigin], func(i, j int) bool {
			return gossiper.messagesMap.messages[wantOrigin][i].ID < gossiper.messagesMap.messages[wantOrigin][j].ID
		})
		gossiper.messagesMap.Unlock()

		gossiper.messagesMap.RLock()
		wantedOriginMessageList := gossiper.messagesMap.messages[wantOrigin]
		gossiper.messagesMap.RUnlock()
		if wantedOriginMessageList != nil {
			for _, message := range wantedOriginMessageList {
				if message.ID >= wantNext {
					return message
				}
			}
		} else {
			continue
		}
	}
	return nil
}

func (gossiper * Gossiper) broadcastToAddr(packet GossipPacket, address string) {
	destinationAddress, _ := net.ResolveUDPAddr("udp", address)

	packetBytes, err := protobuf.Encode(&packet)
	if err != nil {
		panic(err)
	}

	gossiper.gossipConn.WriteToUDP(packetBytes, destinationAddress)
}

func (gossiper * Gossiper) broadcast(packet GossipPacket) {
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
	simple := flag.Bool("simple", false, "run gossiper in simple broadcast mode")

	flag.Parse()

	peersList := strings.Split(*peersString, ",")

	gossiper := NewGossiper(*gossipAddress, *name, peersList, *uiPort, *simple)

	var wg sync.WaitGroup
	wg.Add(2)

	go gossiper.listenUi(&wg)
	go gossiper.listenGossip(&wg)

	wg.Wait()

	fmt.Println(*simple)
}

func NewGossiper(address, name string, peers []string, uiPort string, simple bool) *Gossiper {
	udpAddr, addrErr := net.ResolveUDPAddr("udp4", address)
	if addrErr != nil {
		panic(addrErr)
	}
	udpConn, connErr := net.ListenUDP("udp4", udpAddr)
	if connErr != nil {
		panic(connErr)
	}

	messagesMap := MessagesMap{messages: make(map[string][]RumourMessage)}
	ackWaitList := AckAwaitList{ ackChans: make(map[string]chan StatusPacket)}

	return &Gossiper{
		gossipAddress: udpAddr,
		gossipConn: udpConn,
		Name: name,
		Peers: peers,
		uiPort: uiPort,
		simple: simple,
		ackAwaitList: ackWaitList,
		messagesMap: messagesMap,
	}
}