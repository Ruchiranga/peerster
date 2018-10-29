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
	"os"
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

type StatusPacket struct {
	Want []PeerStatus
}

type GossipPacket struct {
	Simple *SimpleMessage
	Rumor  *RumorMessage
	Status *StatusPacket
}

type Gossiper struct {
	gossipAddress *net.UDPAddr
	gossipConn    *net.UDPConn
	uiPort        string
	Name          string
	simple        bool
	Peers         []string
	ackAwaitList  SyncAckAwaitList
	messagesMap   SyncMessagesMap
	randGen       randGen
	debug         bool
	ticker        *time.Ticker
}

type randGen struct {
	sync.Mutex
 	randGen *rand.Rand
}

type SyncMessagesMap struct {
	sync.Map
}

type SyncAckAwaitList struct {
	sync.Map
}

var (
	Info    *log.Logger
)

func (gossiper *Gossiper) rememberPeer(address string) {
	if address == gossiper.gossipAddress.String() { // Being resilient to other nodes that might have bugs
		return
	}
	for _, str := range gossiper.Peers {
		if str == address {
			return
		}
	}

	channel := make(chan StatusPacket, 1)
	channel <- StatusPacket{}
	gossiper.ackAwaitList.Store(address, &channel)
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
			gossiper.storeMessage(rumour)
			statusPacket := gossiper.generateStatusPacket()
			gossiper.broadcastToAddr(GossipPacket{Status: &statusPacket}, relayPeer)
			gossiper.rumourMonger(*packet.Rumor, relayPeer, false)
		} else {
			if gossiper.debug {
				Info.Printf("__________Rumor %s is not news. So just sending back a status\n", packet.Rumor.Text)
			}
			statusPacket := gossiper.generateStatusPacket()
			gossiper.broadcastToAddr(GossipPacket{Status: &statusPacket}, relayPeer)
		}
	} else if packet.Status != nil {
		statusStr := fmt.Sprintf("STATUS from %s", relayPeer)
		for _, peerStatus := range packet.Status.Want {
			statusStr += fmt.Sprintf(" peer %s nextID %d", peerStatus.Identifier, peerStatus.NextId)
		}
		fmt.Printf("%s\n", statusStr)
		fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

		if gossiper.debug {
			Info.Printf("Awaiting to obtain read lock to try write status received to ack channel of %s\n", relayPeer)
		}

		value, _:= gossiper.ackAwaitList.Load(relayPeer)
		channel := value.(*chan StatusPacket)
		select {
		case *channel <- *packet.Status:
			{
				if gossiper.debug {
					Info.Printf("__________Wrote status received from %s to ack channel\n", relayPeer)
				}
				return
			}
		default:
			{
				if gossiper.debug {
					Info.Printf("__________Processing anti-entropy status from %s\n", relayPeer)
				}
				nextGossipPacket := gossiper.getNextMsgToSend(packet.Status.Want)
				if nextGossipPacket.Rumor != nil {
					if gossiper.debug {
						Info.Println("__________Initiating mongering as a result of anti entropy")
					}
					// In anti-entropy case we know the status sender does not have the message and we need to lock on him
					// when sending the rumor, not send to a random peer.
					gossiper.rumourMonger(*nextGossipPacket.Rumor, relayPeer, true)
				} else if nextGossipPacket.Status != nil {
					if gossiper.debug {
						Info.Printf("__________Sending status to %s as a result of anti entropy\n", relayPeer)
					}
					gossiper.broadcastToAddr(nextGossipPacket, relayPeer)
				} else {
					if gossiper.debug {
						Info.Printf("__________In sync with %s - query from anti entropy\n", relayPeer)
					}
					fmt.Printf("IN SYNC WITH %s\n", relayPeer)
				}
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
				Info.Printf("__________Locked on peer %s for rumourmongering\n", randomPeerAddress)
			}
		} else {
			var randomPeerIdx int

		inner:
			for {
				if gossiper.debug {
					Info.Printf("__________Awaiting to generate random idx %d for rumourmongering\n", randomPeerIdx)
				}
				gossiper.randGen.Lock()
				randomPeerIdx = gossiper.randGen.randGen.Intn(len(gossiper.Peers))
				gossiper.randGen.Unlock()
				if gossiper.debug {
					Info.Printf("__________Generated random idx %d for rumourmongering\n", randomPeerIdx)
				}
				if len(gossiper.Peers) == 1 || gossiper.Peers[randomPeerIdx] != lastMongeredWith {
					break inner
				}
			}

			randomPeerAddress = gossiper.Peers[randomPeerIdx]
		}

		//peerAckChan := make(chan StatusPacket, 1)
		// Assuming peers and ackChans arrays are parallel

		if gossiper.debug {
			Info.Printf("__________Awaiting to obtain lock to clear ack channel and moger with peer %s\n", randomPeerAddress)
		}

		value, _:= gossiper.ackAwaitList.Load(randomPeerAddress)
		channel := value.(*chan StatusPacket)
		<- *channel
		if gossiper.debug {
			Info.Printf("__________Channel cleared to monger with peer %s. length is %d\n", randomPeerAddress,
				len(*channel))
		}

		gossipPacket := GossipPacket{Rumor: &rumourToMonger}

		if flippedCoin {
			fmt.Printf("FLIPPED COIN sending rumor to %s\n", randomPeerAddress)
			flippedCoin = false
		}
		if gossiper.debug {
			Info.Printf("__________Mongering, writing to address %s \n", randomPeerAddress)
		}
		go gossiper.broadcastToAddr(gossipPacket, randomPeerAddress)

		lastMongeredWith = randomPeerAddress

		ticker := time.NewTicker(time.Second)

		select {
		case ackStatus := <- *channel:
			{
				*channel <- StatusPacket{}
				if gossiper.debug {
					Info.Printf("__________Filled the ack chan of %s with dummy\n", randomPeerAddress)
				}
				ticker.Stop()

				if gossiper.debug {
					Info.Printf("__________Received status having wants of length %d from channel\n", len(ackStatus.Want))
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
				if gossiper.debug {
					Info.Printf("__________Timed out waiting for ack chan of %s\n", randomPeerAddress)
				}
				*channel <- StatusPacket{}
				if gossiper.debug {
					Info.Printf("__________Filled the ack chan of %s with dummy\n", randomPeerAddress)
				}
				lockedOnPeer = false

				ticker.Stop()
			}
		}

		if gossiper.debug {
			Info.Printf("__________Awaiting to generate random value for fipping coin\n")
		}
		gossiper.randGen.Lock()
		randomValue := gossiper.randGen.randGen.Int()
		gossiper.randGen.Unlock()

		if gossiper.debug {
			Info.Printf("__________Generated random value %d for fipping coin\n", randomValue)
		}
		i := randomValue % 2

		if i == 0 {
			if gossiper.debug {
				Info.Println("__________Flipped coin but got Tails :(")
			}
			break outer
		} else {
			if gossiper.debug {
				Info.Println("__________Flipped coin and got Heads ^_^")
			}
			flippedCoin = true
			continue outer
		}
	}
}

func (gossiper *Gossiper) generateStatusPacket() (statusPacket StatusPacket) {
	var wantList []PeerStatus
	wantList = []PeerStatus{}
	gossiper.messagesMap.Range(func(peer, messagesList interface{}) bool {
		messages := []RumorMessage{}
		if messagesList != nil {
			messages = *messagesList.(*[]RumorMessage)
		}
		fmt.Println("messages list :", messages)
		nextId := getNextWantId(messages)
		fmt.Println("returned next id : ", nextId)
		wantList = append(wantList, PeerStatus{Identifier: peer.(string), NextId: nextId})
		return true
	})

	if gossiper.debug {
		Info.Printf("__________Want list %v\n", wantList)
	}

	statusPacket = StatusPacket{Want: wantList}
	return
}

func getNextWantId(messages []RumorMessage) (nextId uint32) {
	if len(messages) == 0 {
		return 1
	} else if len(messages) == 1{
		if messages[0].ID == 1 {
			return 2
		} else {
			return 1
		}
	} else {
		nextId = uint32(len(messages) + 1)

		for idx, message := range messages {
			if idx < (len(messages) - 1) {
				if message.ID == messages[idx+1].ID {
					nextId = message.ID + 1
					continue
				} else if message.ID+1 != messages[idx+1].ID {
					nextId = message.ID + 1
					break
				} else {
					nextId = messages[idx+1].ID + 1
				}
			}

		}
	}

	//var idx int
	//for idx = range messages {
	//	if messages[idx].ID == uint32(idx+1) {
	//		continue
	//	}
	//	if messages[idx].ID != uint32(idx+1) {
	//		if idx > 0 && messages[idx - 1].ID != messages[idx].ID {
	//			nextId = uint32(idx + 1)
	//			break
	//		} else {
	//			nextId = uint32(idx + 1)
	//			break
	//		}
	//	}
	//}
	return
}

func (gossiper *Gossiper) storeMessage(rumour *RumorMessage) {
	if gossiper.debug {
		Info.Printf("__________Awaiting to store message %s\n", rumour.Text)
	}

	messages, _ := gossiper.messagesMap.Load(rumour.Origin)

	messageList := []RumorMessage{}
	if messages != nil {
		messageList = *messages.(*[]RumorMessage)
	}

	messageList = append(messageList, *rumour)
	sort.Slice(messageList, func(i, j int) bool {
		return messageList[i].ID < messageList[j].ID
	})

	gossiper.messagesMap.Store(rumour.Origin, &messageList)

	if gossiper.debug {
		Info.Printf("__________Stored message %s\n", rumour.Text)
	}
}

func (gossiper *Gossiper) isRumorNews(rumour *RumorMessage) (news bool) {
	news = true

	if gossiper.debug {
		Info.Printf("__________Awaiting to obtain messages map to check if rumour %s is news\n", rumour.Text)
	}
	messages, _ := gossiper.messagesMap.Load(rumour.Origin)
	if gossiper.debug {
		Info.Printf("__________Obtained messages map to check if rumour %s is news\n", rumour.Text)
	}
	messagesList := []RumorMessage{}
	if messages != nil {
		messagesList = *messages.(*[]RumorMessage)
	}


	//wantId := getNextWantId(messagesList)
	//if rumour.ID != wantId {
	//	news = false
	//}

	for _, message := range messagesList {
		if message.ID == rumour.ID {
			news = false
			break
		}
	}
	return
}

func (gossiper *Gossiper) listenUi(wg *sync.WaitGroup) {
	defer wg.Done()

	messageHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			{
				//gossiper.messagesMap.RLock()
				//messagesMap := gossiper.messagesMap.messages
				//gossiper.messagesMap.RUnlock()
				var tempMap map[string][] RumorMessage
				tempMap = make(map[string][]RumorMessage)
				gossiper.messagesMap.Range(func(peer, messageList interface{}) bool {
					messages := *messageList.(*[]RumorMessage)
					tempMap[peer.(string)] = messages
					return true
				})
				mapJson, err := json.Marshal(tempMap)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

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
				} else {
					fmt.Printf("CLIENT MESSAGE %s\n", packet.Simple.Contents)
				}
				fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))

				if packet.Rumor != nil {
 					messagesList, _ := gossiper.messagesMap.Load(gossiper.Name)
 					messages := []RumorMessage{}
					if messagesList != nil {
						messages = *messagesList.(*[]RumorMessage)
					}

					nextId := getNextWantId(messages)
					packet.Rumor.ID = nextId
					packet.Rumor.Origin = gossiper.Name

					fmt.Println("Storing client rumor with ID ", )
					if gossiper.isRumorNews(packet.Rumor) {
						gossiper.storeMessage(packet.Rumor)
						go gossiper.rumourMonger(*packet.Rumor, "", false)
					}
				} else { // SimpleMessage
					go gossiper.broadcast(packet)
				}
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
				address := string(node[:])
				if gossiper.debug {
					Info.Printf("Awaiting to obtain lock to initialize ack channel of %s\n", address)
				}
				//gossiper.ackAwaitList.RLock()
				channel := make(chan StatusPacket, 1)
				channel <- StatusPacket{}
				gossiper.ackAwaitList.Store(address, &channel)
				gossiper.Peers = append(gossiper.Peers, address)

				if gossiper.debug {
					Info.Printf("Obtained lock and initialized ack channel of %s with a channel len %d\n", address,
						len(channel))
				}
				//gossiper.ackAwaitList.RUnlock()
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

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir("./client/static"))

	mux.Handle("/", fileServer)
	mux.Handle("/message", messageHandler)
	mux.Handle("/node", nodeHandler)
	mux.Handle("/id", idHandler)

	if gossiper.debug {
		Info.Printf("UI Server starting on :%s\n", gossiper.uiPort)
	}
	err := http.ListenAndServe(fmt.Sprintf(":%s", gossiper.uiPort), mux)

	if err != nil {
		panic(err)
	}
}

func (gossiper *Gossiper) getNextMsgToSend(peerWants []PeerStatus) (gossip GossipPacket) {
	gossip = GossipPacket{}

	if gossiper.debug {
		Info.Println("Awaiting to obtain messageMap lock in getNextMsgToSend for peer wants ", peerWants)
	}

	// Check for any news I have
	//gossiper.messagesMap.RLock()

	//if gossiper.debug {
	//	Info.Println("Messages map: ", gossiper.messagesMap.messages)
	//	Info.Println("Peer wants", peerWants)
	//}
	var origins []string
	//var messages []RumorMessage
	gossiper.messagesMap.Range(func(origin, messagesList interface{}) bool {
		//messages = *messagesList.(*[]RumorMessage)
		origins = append(origins, origin.(string))
		return true
	})

outer:
	for _, origin := range origins {
		messagesList, _ := gossiper.messagesMap.Load(origin)
		messages := []RumorMessage{}
		if messagesList != nil {
			messages = *messagesList.(*[]RumorMessage)
		}
		if len(messages) > 0 {
			for _, peerStatus := range peerWants {
				if peerStatus.Identifier == origin {
					peerWant := peerStatus.NextId
					// TODO: simplify with getNextWantId()?
					for _, message := range messages {
						if message.ID >= peerWant {
							gossip = GossipPacket{Rumor: &message}
							if gossiper.debug {
								Info.Println("Returning a RUMOR as next message for peer wants ", peerWants)
							}
							return
						}
					}
					continue outer
				}
			}
			gossip = GossipPacket{Rumor: &(messages[0])}
			if gossiper.debug {
				Info.Println("Returning a RUMOR as next message for peer wants ", peerWants)
			}
			return
		}
	}

	// Execution coming here means I have no news to send. So check if peer has news

	if gossiper.debug {
		Info.Println("I have no news to send. So check if peer has news")
	}

	for _, peerStatus := range peerWants {
		if peerStatus.NextId == 1 {
			continue
		}

		peerId := peerStatus.Identifier
		messagesList, _ := gossiper.messagesMap.Load(peerId)
		messages := []RumorMessage{}
		if messagesList != nil {
			messages = *messagesList.(*[]RumorMessage)
		}
		if getNextWantId(messages) < peerStatus.NextId {
			statusPacket := gossiper.generateStatusPacket()
			//gossiper.messagesMap.RUnlock()
			gossip = GossipPacket{Status: &statusPacket}
			if gossiper.debug {
				Info.Println("Returning a STATUS as next message for peer wants ", peerWants)
			}
			return
		}

	}

	if gossiper.debug {
		Info.Println("None of us have news.")
	}
	// Execution coming here means neither of us have news.
	//gossiper.messagesMap.RUnlock()
	if gossiper.debug {
		Info.Println("Returning NOTHING as next message for peer wants ", peerWants)
	}
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
	defer gossiper.ticker.Stop()

	for range gossiper.ticker.C {
		if len(gossiper.Peers) > 0 {
			if gossiper.debug {
				Info.Println("Awaiting to obtain lock to generate random int to do anti entropy")
			}
			gossiper.randGen.Lock()
			randomPeerIdx := gossiper.randGen.randGen.Intn(len(gossiper.Peers))
			gossiper.randGen.Unlock()
			if gossiper.debug {
				Info.Println("Obtained lock and generated random int to do anti entropy ", randomPeerIdx)
			}
			statusPacket := gossiper.generateStatusPacket()
			gossipPacket := GossipPacket{Status: &statusPacket}
			go gossiper.broadcastToAddr(gossipPacket, gossiper.Peers[randomPeerIdx])
		}
	}
}

func main() {
	DEBUG := false
	Info = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

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
	wg.Add(3)

	go gossiper.listenUi(&wg)
	go gossiper.listenGossip(&wg)
	go gossiper.doAntiEntropy(&wg)

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

	messagesMap := SyncMessagesMap{}
	//messagesMap := MessagesMap{messages: make(map[string][]RumorMessage)}
	//ackWaitList := AckAwaitList{ackChans: make(map[string]chan StatusPacket)}
	ackWaitList := SyncAckAwaitList{}

	for _, peer := range peers {
		channel := make(chan StatusPacket, 1)
		channel <- StatusPacket{}
		ackWaitList.Store(peer, &channel)
		//if debug {
		//	Info.Printf("Initialized ack channel of %s with a channel len %d\n", peer,
		//		len(ackWaitList.ackChans[address]))
		//}
	}

	randSource := rand.NewSource(time.Now().UTC().UnixNano())
	randGenerator := rand.New(randSource)

	randGen := randGen{randGen: randGenerator}

	ticker := time.NewTicker(time.Second)

	return &Gossiper{
		gossipAddress: udpAddr,
		gossipConn:    udpConn,
		Name:          name,
		Peers:         peers,
		uiPort:        uiPort,
		simple:        simple,
		ackAwaitList:  ackWaitList,
		messagesMap:   messagesMap,
		randGen:       randGen,
		debug:         debug,
		ticker:        ticker,
	}
}
