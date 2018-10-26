package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dedis/protobuf"
	"io/ioutil"
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
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

type RumorMessage struct {
	Origin string
	ID     uint32
	Text   string
}

type PeerStatus struct {
	Identifier string
	NextId     uint32
}

type PrivateMessage struct {
	Origin      string
	ID          uint32
	Text        string
	Destination string
	HopLimit    uint32
}

type GenericMessage struct {
	Origin string
	ID     uint32
	Text   string
}

type StatusPacket struct {
	Want []PeerStatus
}

type GossipPacket struct {
	Simple  *SimpleMessage
	Rumor   *RumorMessage
	Status  *StatusPacket
	Private *PrivateMessage
}

type Gossiper struct {
	gossipAddress *net.UDPAddr
	gossipConn    *net.UDPConn
	uiPort        string
	Name          string
	simple        bool
	Peers         []string
	ackAwaitList  AckAwaitList
	messagesMap   MessagesMap
	privateMap    PrivateMap
	routingTable  RoutingTable
	randGen       *rand.Rand
	debug         bool
	entropyTicker *time.Ticker
	routingTicker *time.Ticker
}

type MessagesMap struct {
	sync.RWMutex
	messages map[string][]RumorMessage
}

type PrivateMap struct {
	sync.RWMutex
	messages map[string][]PrivateMessage
}

type RoutingTable struct {
	sync.RWMutex
	table map[string]string
}

type AckAwaitList struct {
	sync.RWMutex
	ackChans map[string]chan StatusPacket
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
		_, relayPeer, err := gossiper.gossipConn.ReadFromUDP(packetBytes)

		if err != nil {
			log.Fatal(err)
			continue
		}

		protobuf.Decode(packetBytes, &packet)
		gossiper.rememberPeer(relayPeer.String())

		if gossiper.simple && packet.Simple != nil {
			fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", packet.Simple.OriginalName, packet.Simple.RelayPeerAddr, packet.Simple.Contents)
			fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

			go gossiper.broadcast(packet)
		} else {
			go gossiper.handleGossip(packet, relayPeer.String())
		}

	}
}

func (gossiper *Gossiper) handleGossip(packet GossipPacket, relayPeer string) {
	if packet.Rumor != nil {
		fmt.Printf("RUMOR origin %s from %s ID %d contents %s\n", packet.Rumor.Origin, relayPeer,
			packet.Rumor.ID, packet.Rumor.Text)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		rumour := packet.Rumor

		if gossiper.isRumorNews(rumour) {
			// Update our routing table since this is news
			gossiper.routingTable.Lock()
			gossiper.routingTable.table[rumour.Origin] = relayPeer
			gossiper.routingTable.Unlock()
			fmt.Printf("DSDV %s %s\n", rumour.Origin, relayPeer)

			gossiper.storeMessage(rumour)
			statusPacket := gossiper.generateStatusPacket()
			go gossiper.broadcastToAddr(GossipPacket{Status: &statusPacket}, relayPeer)
			go gossiper.rumourMonger(*packet.Rumor, relayPeer, false)
		} else if gossiper.debug {
			fmt.Printf("__________Rumor %s is not news. So just sending back a status\n", packet.Rumor.Text)
			statusPacket := gossiper.generateStatusPacket()
			go gossiper.broadcastToAddr(GossipPacket{Status: &statusPacket}, relayPeer)
		}
	} else if packet.Status != nil {
		statusStr := fmt.Sprintf("STATUS from %s", relayPeer)
		for _, peerStatus := range packet.Status.Want {
			statusStr += fmt.Sprintf(" peer %s nextID %d", peerStatus.Identifier, peerStatus.NextId)
		}
		fmt.Printf("%s\n", statusStr)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		gossiper.ackAwaitList.RLock()
		if gossiper.ackAwaitList.ackChans[relayPeer] != nil { // If the status is an ack
			if gossiper.debug {
				fmt.Printf("__________Writing status from %s to ack channel\n", relayPeer)
			}

			gossiper.ackAwaitList.ackChans[relayPeer] <- *packet.Status
			gossiper.ackAwaitList.RUnlock()
		} else { // If the status is from anti-entropy
			gossiper.ackAwaitList.RUnlock()

			if gossiper.debug {
				fmt.Printf("__________Processing anti-entropy status from %s\n", relayPeer)
			}
			nextGossipPacket := gossiper.getNextMsgToSend(packet.Status.Want)
			if nextGossipPacket.Rumor != nil {
				if gossiper.debug {
					fmt.Println("__________Initiating mongering as a result of anti entropy")
				}
				// In anti-entropy case we know the status sender does not have the message and we need to lock on him
				// when sending the rumor, not send to a random peer.
				go gossiper.rumourMonger(*nextGossipPacket.Rumor, relayPeer, true)
			} else if nextGossipPacket.Status != nil {
				if gossiper.debug {
					fmt.Printf("__________Sending status to %s as a result of anti entropy\n", relayPeer)
				}
				go gossiper.broadcastToAddr(nextGossipPacket, relayPeer)
			} else {
				if gossiper.debug {
					fmt.Printf("__________In sync with %s - query from anti entropy\n", relayPeer)
				}
				fmt.Printf("IN SYNC WITH %s\n", relayPeer)
			}
		}
	} else if packet.Private != nil {
		if packet.Private.Destination == gossiper.Name {
			fmt.Printf("PRIVATE origin %s hop-limit %d contents %s\n", packet.Private.Origin, packet.Private.HopLimit, packet.Private.Text)
			gossiper.storePrivateMessage(packet.Private)
		} else {
			packet.Private.HopLimit -= 1
			if packet.Private.HopLimit > 0 {
				go gossiper.forwardPrivateMessage(packet.Private)
			}
		}
	} else {
		log.Fatal("Unexpected gossip packet type.")
	}
}

func (gossiper *Gossiper) rumourMonger(rumour RumorMessage, fromPeerAddress string, lockOn bool) {
	rumourToMonger := rumour
	lastMongeredWith := fromPeerAddress
	lockedOnPeer := lockOn
	flippedCoin := false

outer:
	for {
		var randomPeerAddress string

		if lockedOnPeer {
			randomPeerAddress = lastMongeredWith
			if gossiper.debug {
				fmt.Printf("__________Locked on peer %s for rumourmongering\n", randomPeerAddress)
			}
		} else {
			var randomPeerIdx int

		inner:
			for {
				randomPeerIdx = gossiper.randGen.Intn(len(gossiper.Peers))
				if gossiper.debug {
					fmt.Printf("__________Generated random idx %d for rumourmongering\n", randomPeerIdx)
				}
				if len(gossiper.Peers) == 1 || gossiper.Peers[randomPeerIdx] != lastMongeredWith {
					break inner
				}
			}

			randomPeerAddress = gossiper.Peers[randomPeerIdx]
		}

		peerAckChan := make(chan StatusPacket, 1)
		// Assuming peers and ackChans arrays are parallel

		if gossiper.debug {
			fmt.Printf("__________Awaiting to obtain lock to set ack channel and moger with peer %s\n", randomPeerAddress)
		}
		gossiper.ackAwaitList.Lock()
		if gossiper.debug {
			fmt.Printf("__________Obtained lock to set ack channel and moNger with peer %s\n", randomPeerAddress)
		}
		gossiper.ackAwaitList.ackChans[randomPeerAddress] = peerAckChan
		gossiper.ackAwaitList.Unlock()

		gossipPacket := GossipPacket{Rumor: &rumourToMonger}

		if flippedCoin {
			fmt.Printf("FLIPPED COIN sending rumor to %s\n", randomPeerAddress)
			flippedCoin = false
		}
		if gossiper.debug {
			fmt.Printf("__________Mongering, writing to address %s \n", randomPeerAddress)
		}
		go gossiper.broadcastToAddr(gossipPacket, randomPeerAddress)

		lastMongeredWith = randomPeerAddress

		ticker := time.NewTicker(time.Second)

		select {
		case ackStatus := <-peerAckChan:
			{
				gossiper.ackAwaitList.Lock()
				gossiper.ackAwaitList.ackChans[randomPeerAddress] = nil
				if gossiper.debug {
					fmt.Printf("__________Reseted the ack chan of %s to nil\n", randomPeerAddress)
				}
				gossiper.ackAwaitList.Unlock()

				ticker.Stop()

				if gossiper.debug {
					fmt.Printf("__________Received status having wants of length %d from channel\n", len(ackStatus.Want))
				}
				nextMessage := gossiper.getNextMsgToSend(ackStatus.Want)
				if nextMessage.Rumor != nil {
					rumourToMonger = *nextMessage.Rumor
					lockedOnPeer = true
					continue outer
				} else if nextMessage.Status != nil {
					lockedOnPeer = false
					go gossiper.broadcastToAddr(nextMessage, randomPeerAddress)
					break outer
				} else {
					lockedOnPeer = false
					fmt.Printf("IN SYNC WITH %s\n", randomPeerAddress)
				}
			}
		case <-ticker.C:
			{
				gossiper.ackAwaitList.Lock()
				gossiper.ackAwaitList.ackChans[randomPeerAddress] = nil
				if gossiper.debug {
					fmt.Printf("__________Reset the ack chan of %s to nil\n", randomPeerAddress)
				}
				gossiper.ackAwaitList.Unlock()
				lockedOnPeer = false

				ticker.Stop()
			}
		}

		randomValue := gossiper.randGen.Int()
		if gossiper.debug {
			fmt.Printf("__________Generated random value %d for fipping coin\n", randomValue)
		}
		i := randomValue % 2

		if i == 0 {
			if gossiper.debug {
				fmt.Println("__________Flipped coin but got Tails :(")
			}
			break outer
		} else {
			if gossiper.debug {
				fmt.Println("__________Flipped coin and got Heads ^_^")
			}
			flippedCoin = true
			continue outer
		}
	}
}

func (gossiper *Gossiper) generateStatusPacket() (statusPacket StatusPacket) {
	var wantList []PeerStatus
	wantList = []PeerStatus{}

	gossiper.messagesMap.RLock()

	for peer, messages := range gossiper.messagesMap.messages {
		nextId := getNextWantId(messages)
		wantList = append(wantList, PeerStatus{Identifier: peer, NextId: nextId})
	}

	gossiper.messagesMap.RUnlock()

	if gossiper.debug {
		fmt.Printf("__________Want list %v\n", wantList)
	}

	statusPacket = StatusPacket{Want: wantList}
	return
}

func getNextWantId(messages []RumorMessage) (nextId uint32) {
	nextId = uint32(len(messages) + 1)
	var idx int
	for idx = range messages {
		if messages[idx].ID != uint32(idx+1) {
			nextId = uint32(idx + 1)
			break
		}
	}
	return
}

func (gossiper *Gossiper) storeMessage(rumour *RumorMessage) {
	gossiper.messagesMap.Lock()
	messageList := gossiper.messagesMap.messages[rumour.Origin]

	messageList = append(messageList, *rumour)
	sort.Slice(messageList, func(i, j int) bool {
		return messageList[i].ID < messageList[j].ID
	})

	gossiper.messagesMap.messages[rumour.Origin] = messageList
	gossiper.messagesMap.Unlock()
}

func (gossiper *Gossiper) storePrivateMessage(private *PrivateMessage) {
	gossiper.privateMap.Lock()
	messageList := gossiper.privateMap.messages[private.Origin]

	messageList = append(messageList, *private)

	gossiper.privateMap.messages[private.Origin] = messageList
	gossiper.privateMap.Unlock()
}

func (gossiper *Gossiper) isRumorNews(rumour *RumorMessage) (news bool) {
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

func (gossiper *Gossiper) initiateRumorMonger(packet GossipPacket) {
	gossiper.messagesMap.RLock()

	messages := gossiper.messagesMap.messages[gossiper.Name]
	nextId := getNextWantId(messages)

	gossiper.messagesMap.RUnlock()

	packet.Rumor.ID = nextId
	packet.Rumor.Origin = gossiper.Name

	gossiper.storeMessage(packet.Rumor)
	go gossiper.rumourMonger(*packet.Rumor, "", false)
}

func (gossiper *Gossiper) listenUi(wg *sync.WaitGroup) {
	defer wg.Done()

	messageHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			{
				gossiper.messagesMap.RLock()

				messagesMap := gossiper.messagesMap.messages
				mapJson, err := json.Marshal(messagesMap)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				gossiper.messagesMap.RUnlock()

				w.Header().Set("Content-Type", "application/json")
				w.Write(mapJson)
			}
		case http.MethodPost:
			{
				decoder := json.NewDecoder(r.Body)
				var packet GossipPacket
				err := decoder.Decode(&packet)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if packet.Rumor != nil {
					fmt.Printf("CLIENT MESSAGE %s\n", packet.Rumor.Text)
				} else if packet.Private != nil {
					fmt.Printf("CLIENT MESSAGE %s\n", packet.Private.Text)
				} else {
					fmt.Printf("CLIENT MESSAGE %s\n", packet.Simple.Contents)
				}
				fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

				if packet.Rumor != nil {
					gossiper.initiateRumorMonger(packet)
				} else if packet.Private != nil {
					packet.Private.Origin = gossiper.Name
					packet.Private.ID = 0
					packet.Private.HopLimit = 10

					gossiper.routingTable.RLock()

					table := gossiper.routingTable.table
					gossiper.broadcastToAddr(packet, table[packet.Private.Destination])

					gossiper.routingTable.RUnlock()
				} else { // SimpleMessage
					go gossiper.broadcast(packet)
				}
			}
		default:
			http.Error(w, "Unsupported request method.", 405)
		}
	})

	privateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			{
				gossiper.privateMap.RLock()

				messagesMap := gossiper.privateMap.messages
				mapJson, err := json.Marshal(messagesMap)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				gossiper.privateMap.RUnlock()

				w.Header().Set("Content-Type", "application/json")
				w.Write(mapJson)
			}
		default:
			http.Error(w, "Unsupported request method.", 405)
		}
	})

	nodeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			{
				peersList := gossiper.Peers

				listJson, err := json.Marshal(peersList)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write(listJson)
			}
		case http.MethodPost:
			{
				node, err := ioutil.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				gossiper.Peers = append(gossiper.Peers, string(node[:]))
			}
		default:
			http.Error(w, "Unsupported request method.", 405)
		}
	})

	idHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			{
				w.Write([]byte(gossiper.Name))
			}
		default:
			http.Error(w, "Unsupported request method.", 405)
		}
	})

	originsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			{
				gossiper.routingTable.RLock()

				table := gossiper.routingTable.table

				originsList := make([]string, 0, len(table))
				for origin := range table {
					originsList = append(originsList, origin)
				}

				gossiper.routingTable.RUnlock()

				listJson, err := json.Marshal(originsList)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write(listJson)
			}
		default:
			http.Error(w, "Unsupported request method.", 405)
		}
	})

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir("./client/static"))

	mux.Handle("/", fileServer)
	mux.Handle("/message", messageHandler)
	mux.Handle("/node", nodeHandler)
	mux.Handle("/id", idHandler)
	mux.Handle("/origins", originsHandler)
	mux.Handle("/private", privateHandler)

	if gossiper.debug {
		fmt.Printf("UI Server starting on :%s\n", gossiper.uiPort)
	}
	err := http.ListenAndServe(fmt.Sprintf(":%s", gossiper.uiPort), mux)

	if err != nil {
		panic(err)
	}
}

func (gossiper *Gossiper) getNextMsgToSend(peerWants []PeerStatus) (gossip GossipPacket) {
	gossip = GossipPacket{}

	// Check for any news I have
	gossiper.messagesMap.RLock()

outer:
	for origin := range gossiper.messagesMap.messages {
		if len(gossiper.messagesMap.messages[origin]) > 0 {
			for _, peerStatus := range peerWants {
				if peerStatus.Identifier == origin {
					peerWant := peerStatus.NextId
					// TODO: simplify with getNextWantId()?
					for _, message := range gossiper.messagesMap.messages[origin] {
						if message.ID >= peerWant {
							gossip = GossipPacket{Rumor: &message}
							gossiper.messagesMap.RUnlock()
							return
						}
					}
					continue outer
				}
			}
			gossip = GossipPacket{Rumor: &gossiper.messagesMap.messages[origin][0]}
			gossiper.messagesMap.RUnlock()
			return
		}
	}

	// Execution coming here means I have no news to send. So check if peer has news

	for _, peerStatus := range peerWants {
		if peerStatus.NextId == 1 {
			continue
		}

		peerId := peerStatus.Identifier
		if getNextWantId(gossiper.messagesMap.messages[peerId]) < peerStatus.NextId {
			statusPacket := gossiper.generateStatusPacket()
			gossip = GossipPacket{Status: &statusPacket}
			gossiper.messagesMap.RUnlock()
			return
		}

	}

	// Execution coming here means neither of us have news.
	gossiper.messagesMap.RUnlock()
	return
}

func (gossiper *Gossiper) broadcastToAddr(packet GossipPacket, address string) {
	if packet.Rumor != nil {
		fmt.Printf("MONGERING with %s\n", address)
	}

	destinationAddress, _ := net.ResolveUDPAddr("udp", address)

	packetBytes, err := protobuf.Encode(&packet)
	if err != nil {
		panic(err)
	}

	gossiper.gossipConn.WriteToUDP(packetBytes, destinationAddress)
}

func (gossiper *Gossiper) forwardPrivateMessage(private *PrivateMessage) {
	gossiper.routingTable.RLock()

	table := gossiper.routingTable.table
	address := table[private.Destination]

	gossiper.routingTable.RUnlock()
	packet := GossipPacket{Private: private}
	gossiper.broadcastToAddr(packet, address)
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

func (gossiper *Gossiper) doAntiEntropy(wg *sync.WaitGroup) {
	defer wg.Done()
	defer gossiper.entropyTicker.Stop()

	for range gossiper.entropyTicker.C {
		if len(gossiper.Peers) > 0 {
			randomPeerIdx := gossiper.randGen.Intn(len(gossiper.Peers))
			statusPacket := gossiper.generateStatusPacket()
			gossipPacket := GossipPacket{Status: &statusPacket}
			go gossiper.broadcastToAddr(gossipPacket, gossiper.Peers[randomPeerIdx])
		}
	}
}

func (gossiper *Gossiper) doRouteRumour(wg *sync.WaitGroup) {
	defer wg.Done()
	defer gossiper.routingTicker.Stop()

	packet := GossipPacket{Rumor: &RumorMessage{Text: ""}}

	for ; true; <- gossiper.routingTicker.C {
		if gossiper.debug {
			fmt.Println("__________Sending out a route rumour")
		}
		gossiper.initiateRumorMonger(packet)
	}
}

func main() {
	DEBUG := true

	uiPort := flag.String("UIPort", "8080", "Port for the UI client")
	gossipAddress := flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
	name := flag.String("name", "node-ruchiranga", "Name of the gossiper")
	peersString := flag.String("peers", "127.0.0.1:5001,10.1.1.7:5002", "comma separated list of peers of the form ip:port")
	simple := flag.Bool("simple", false, "run gossiper in simple broadcast mode")
	rtimer := flag.Int("rtimer", 0 , "route rumors sending period in seconds, 0 to disable sending of route rumors")
	flag.Parse()

	var peersList []string
	if *peersString == "" {
		peersList = []string{}
	} else {
		peersList = strings.Split(*peersString, ",")
	}

	gossiper := NewGossiper(*gossipAddress, *name, peersList, *uiPort, *simple, *rtimer, DEBUG)

	var wg sync.WaitGroup
	wg.Add(4)

	go gossiper.listenUi(&wg)
	go gossiper.listenGossip(&wg)
	go gossiper.doAntiEntropy(&wg)
	if *rtimer > 0 {
		go gossiper.doRouteRumour(&wg)
	}

	wg.Wait()
}

func NewGossiper(address, name string, peers []string, uiPort string, simple bool, rtimer int, debug bool) *Gossiper {
	udpAddr, addrErr := net.ResolveUDPAddr("udp4", address)
	if addrErr != nil {
		panic(addrErr)
	}
	udpConn, connErr := net.ListenUDP("udp4", udpAddr)
	if connErr != nil {
		panic(connErr)
	}

	messagesMap := MessagesMap{messages: make(map[string][]RumorMessage)}
	privateMap := PrivateMap{messages: make(map[string][]PrivateMessage)}
	routingTable := RoutingTable{table: make(map[string]string)}
	ackWaitList := AckAwaitList{ackChans: make(map[string]chan StatusPacket)}

	randSource := rand.NewSource(time.Now().UTC().UnixNano())
	randGen := rand.New(randSource)

	entropyTicker := time.NewTicker(time.Second)
	var routingTicker *time.Ticker
	if rtimer > 0 {
		routingTicker = time.NewTicker(time.Duration(rtimer) * time.Second)
	}

	return &Gossiper{
		gossipAddress: udpAddr,
		gossipConn:    udpConn,
		Name:          name,
		Peers:         peers,
		uiPort:        uiPort,
		simple:        simple,
		ackAwaitList:  ackWaitList,
		messagesMap:   messagesMap,
		privateMap:    privateMap,
		randGen:       randGen,
		debug:         debug,
		entropyTicker: entropyTicker,
		routingTicker: routingTicker,
		routingTable:  routingTable,
	}
}
