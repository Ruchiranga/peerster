package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/dedis/protobuf"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type Gossiper struct {
	jobsChannel      chan func()
	gossipAddress    *net.UDPAddr
	gossipConn       *net.UDPConn
	uiPort           string
	Name             string
	simple           bool
	Peers            []string
	ackAwaitMap      map[string]func(status StatusPacket)
	fileAwaitMap     map[string]func(reply DataReply)
	messagesMap      map[string][]GenericMessage
	routingTable     map[string]string
	randGen          *rand.Rand
	debug            bool
	entropyTicker    *time.Ticker
	routingTicker    *time.Ticker
	fileList         []FileIndex
	fileContentMap   map[string][]byte
	currentDownloads map[string][]byte
}

type FileIndex struct {
	Name     string
	Size     uint32
	MetaFile []byte
	MetaHash [32]byte
}

func (gossiper *Gossiper) printPeers() {
	//fmt.Printf("PEERS %s\n", strings.Join(gossiper.Peers, ","))
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

		packetBytes := make([]byte, 16384) //16KB
		_, relayPeer, err := gossiper.gossipConn.ReadFromUDP(packetBytes)

		if err != nil {
			log.Fatal(err)
			continue
		}

		protobuf.Decode(packetBytes, &packet)
		gossiper.jobsChannel <- func() {
			gossiper.rememberPeer(relayPeer.String())

			if gossiper.simple && packet.Simple != nil {
				printSimpleMessageLog(*packet.Simple)
				gossiper.printPeers()

				gossiper.broadcast(packet)
			} else {
				gossiper.handleGossip(packet, relayPeer.String())
			}
		}
	}
}

func (gossiper *Gossiper) storeNextHop(origin string, nextHop string) {
	printDSDVLog(origin, nextHop)
	gossiper.routingTable[origin] = nextHop
}

func (gossiper *Gossiper) requestFileChunk(metaHashHex string, metaFile []byte, fromIndex int, fileName string, destination string, done chan bool) {
	if fromIndex == len(metaFile) {
		done <- true
		return
	}

	endIndex := fromIndex + 32
	if endIndex > len(metaFile) {
		endIndex = len(metaFile)
	}
	hashChunk := metaFile[fromIndex:endIndex]

	dataReplyChan := make(chan DataReply)
	hashChunkHex := hex.EncodeToString(hashChunk)
	gossiper.fileAwaitMap[hashChunkHex] = func(reply DataReply) {
		select {
		case dataReplyChan <- reply:
		default:
		}
	}

	dataRequest := DataRequest{Origin: gossiper.Name, Destination: destination, HopLimit: 10, HashValue: hashChunk}
	success := gossiper.forwardDataRequest(&dataRequest)

	printFileChunkDownloadLog(fileName, (fromIndex/32)+1, destination)
	if success {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			var replyPtr *DataReply
			replyPtr = nil
			select {
			case reply := <-dataReplyChan:
				replyPtr = &reply
			case <-ticker.C:
			}

			gossiper.jobsChannel <- func() {
				close(dataReplyChan)
				delete(gossiper.fileAwaitMap, hashChunkHex)
				ticker.Stop()

				if replyPtr != nil {
					fmt.Println("Got SOME reply")
					replyDataHash := sha256.Sum256(replyPtr.Data)
					if bytes.Equal(replyPtr.HashValue, replyDataHash[:]) {
						fmt.Println("reply is not valid")
						// To be able to be served to another peer
						gossiper.fileContentMap[hashChunkHex] = replyPtr.Data
						downloadedChunks := gossiper.currentDownloads[metaHashHex] // Should exist for sure
						gossiper.currentDownloads[metaHashHex] = append(downloadedChunks, replyPtr.Data...)
						fmt.Println("downloadedChunks len", len(downloadedChunks))
						gossiper.requestFileChunk(metaHashHex, metaFile, endIndex, fileName, destination, done)
						return
					}
					rdhash := sha256.Sum256(replyPtr.Data)
					fmt.Println("bytes.Equal(replyPtr.HashValue, replyDataHash[:])", bytes.Equal(replyPtr.HashValue, rdhash[:]))
				}
				fmt.Println("replyptr nil? ", replyPtr == nil)

				// keep retrying if I didn't get what I want
				gossiper.requestFileChunk(metaHashHex, metaFile, fromIndex, fileName, destination, done)
			}
		}()
	}
}

func (gossiper *Gossiper) initiateFileDownload(metaHashHex string, fileName string, destination string, done chan bool) {
	metaHash, err := hex.DecodeString(metaHashHex)
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to decode hex string %s to []byte", metaHashHex))
		return
	}

	dataReplyChan := make(chan DataReply)
	gossiper.fileAwaitMap[metaHashHex] = func(reply DataReply) {
		select {
		case dataReplyChan <- reply:
		default:
		}
	}

	dataRequest := DataRequest{Origin: gossiper.Name, Destination: destination, HopLimit: 10, HashValue: metaHash}
	success := gossiper.forwardDataRequest(&dataRequest)

	printMetaFileDownloadLog(fileName, destination)
	if success {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			var replyPtr *DataReply
			replyPtr = nil
			select {
			case reply := <-dataReplyChan:
				replyPtr = &reply
			case <-ticker.C:
			}

			gossiper.jobsChannel <- func() {
				close(dataReplyChan)
				delete(gossiper.fileAwaitMap, metaHashHex)
				ticker.Stop()

				if replyPtr != nil {
					replyDataHash := sha256.Sum256(replyPtr.Data)
					if bytes.Equal(replyPtr.HashValue, replyDataHash[:]) {
						// To be able to be served to another peer
						gossiper.fileContentMap[metaHashHex] = replyPtr.Data
						gossiper.requestFileChunk(metaHashHex, replyPtr.Data, 0, fileName, destination, done)
						return
					}
				}
				// keep retrying if I didn't get what I want
				gossiper.initiateFileDownload(metaHashHex, fileName, destination, done)
			}
		}()
	}
}

func (gossiper *Gossiper) requestFile(metaHashHex string, destination string, saveAs string) {
	gossiper.currentDownloads[metaHashHex] = []byte{}
	done := make(chan bool)
	fmt.Println("Calling initiateFileDownload with ", metaHashHex, saveAs, destination, done)
	gossiper.initiateFileDownload(metaHashHex, saveAs, destination, done)

	go func() {
		<-done
		content := gossiper.currentDownloads[metaHashHex]
		writeToFile(content, saveAs)
		printFileReconstructLog(saveAs)
		delete(gossiper.currentDownloads, metaHashHex)
	}()
}

func (gossiper *Gossiper) handleGossip(packet GossipPacket, relayPeer string) {
	if packet.Rumor != nil {
		printRumorMessageLog(*packet.Rumor, relayPeer)
		gossiper.printPeers()

		rumor := packet.Rumor

		if gossiper.isRumorNews(rumor) {
			if gossiper.debug {
				fmt.Printf("__________Rumor %s is news. Proceeding to store message\n", packet.Rumor.Text)
			}
			message := getGenericMessageFromRumor(*rumor)
			gossiper.storeMessage(message)
			gossiper.storeNextHop(rumor.Origin, relayPeer)
			statusPacket := gossiper.generateStatusPacket()
			gossiper.writeToAddr(GossipPacket{Status: &statusPacket}, relayPeer)
			gossiper.rumorMonger(*packet.Rumor, relayPeer, "", false)
		} else {
			if gossiper.debug {
				fmt.Printf("__________Rumor %s is not news. So just sending back a status\n", packet.Rumor.Text)
			}
			statusPacket := gossiper.generateStatusPacket()
			gossiper.writeToAddr(GossipPacket{Status: &statusPacket}, relayPeer)
		}
	} else if packet.Status != nil {
		printStatusMessageLog(packet, relayPeer)
		gossiper.printPeers()

		handler, available := gossiper.ackAwaitMap[relayPeer]
		if available {
			handler(*packet.Status)
		} else { // If the status is from anti-entropy
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
				gossiper.rumorMonger(*nextGossipPacket.Rumor, relayPeer, "", true)
			} else if nextGossipPacket.Status != nil {
				if gossiper.debug {
					fmt.Printf("__________Sending status to %s as a result of anti entropy\n", relayPeer)
				}
				gossiper.writeToAddr(nextGossipPacket, relayPeer)
			} else {
				printInSyncLog(relayPeer)
			}
		}
	} else if packet.Private != nil {
		if packet.Private.Destination == gossiper.Name {
			printPrivateMessageLog(*packet.Private)
			genericMessage := getGenericMessageFromPrivate(*packet.Private)
			gossiper.storeMessage(genericMessage)
		} else {
			packet.Private.HopLimit -= 1
			if packet.Private.HopLimit > 0 {
				gossiper.forwardPrivateMessage(packet.Private)
			}
		}
	} else if packet.DataRequest != nil {
		fmt.Println("DataRequest received", packet.DataRequest.Destination, packet.DataRequest.Origin, packet.DataRequest.HashValue, packet.DataRequest.HopLimit)
		request := packet.DataRequest
		if request.Destination == gossiper.Name {
			data, available := gossiper.fileContentMap[hex.EncodeToString(request.HashValue)]
			fmt.Println("reply available", available)
			fmt.Println("reply", data)

			if available {
				reply := DataReply{Origin: gossiper.Name, Destination: request.Origin, HopLimit: 10, HashValue: request.HashValue, Data: data}
				gossiper.forwardDataReply(&reply)
			}
		} else {
			packet.DataRequest.HopLimit -= 1
			if packet.DataRequest.HopLimit > 0 {
				gossiper.forwardDataRequest(packet.DataRequest)
			}
		}
	} else if packet.DataReply != nil {
		fmt.Println("DataReply received", packet.DataReply.Destination, packet.DataReply.Origin, packet.DataReply.HashValue, packet.DataReply.HopLimit)

		if packet.DataReply.Destination == gossiper.Name {
			handler, available := gossiper.fileAwaitMap[hex.EncodeToString(packet.DataReply.HashValue)]
			fmt.Println("hadnle available", available)
			if available {
				handler(*packet.DataReply)
			}
		} else {
			packet.DataReply.HopLimit -= 1
			if packet.DataReply.HopLimit > 0 {
				gossiper.forwardDataReply(packet.DataReply)
			}
		}
	} else {
		log.Fatal("Unexpected gossip packet type.")
	}
}

func (gossiper *Gossiper) forwardPrivateMessage(private *PrivateMessage) {
	address, found := gossiper.routingTable[private.Destination]
	if found {
		packet := GossipPacket{Private: private}
		gossiper.writeToAddr(packet, address)
	} else {
		log.Fatal(fmt.Sprintf("Failed to forward private message. Next hop for %s not found.", private.Destination))
	}

}

func (gossiper *Gossiper) forwardDataRequest(request *DataRequest) (success bool) {
	address, found := gossiper.routingTable[request.Destination]
	if found {
		packet := GossipPacket{DataRequest: request}
		gossiper.writeToAddr(packet, address)
		fmt.Println("Wrote date request to", address)
		return true
	} else {
		log.Fatal(fmt.Sprintf("Failed to forward data request. Next hop for %s not found.", request.Destination))
		return false
	}

}

func (gossiper *Gossiper) forwardDataReply(reply *DataReply) (success bool) {
	address, found := gossiper.routingTable[reply.Destination]
	if found {
		fmt.Println("forwarding data reply to", address, "reply data ", reply.Origin, reply.Destination, reply.HopLimit)
		packet := GossipPacket{DataReply: reply}
		gossiper.writeToAddr(packet, address)
		fmt.Println("Wrote date reply to", address)

		return true
	} else {
		log.Fatal(fmt.Sprintf("Failed to forward data reply. Next hop for %s not found.", reply.Destination))
		return false
	}

}

func (gossiper *Gossiper) rumorMonger(rumour RumorMessage, fromPeerAddress string, lockedOnPeer string, coinFlipped bool) {
	var randomPeerAddress string

	if lockedOnPeer != "" {
		randomPeerAddress = lockedOnPeer
		if gossiper.debug {
			fmt.Printf("__________Locked on peer %s for rumourmongering\n", randomPeerAddress)
		}
	} else {
		var randomPeerIdx int

		for {
			randomPeerIdx = gossiper.randGen.Intn(len(gossiper.Peers))
			if gossiper.debug {
				fmt.Printf("__________Generated random idx %d for rumourmongering\n", randomPeerIdx)
			}
			if len(gossiper.Peers) == 1 || gossiper.Peers[randomPeerIdx] != fromPeerAddress {
				break
			}
		}

		randomPeerAddress = gossiper.Peers[randomPeerIdx]
	}

	if coinFlipped {
		printCoinFlippedLog(randomPeerAddress)
	}

	peerAckChan := make(chan StatusPacket)

	gossiper.ackAwaitMap[randomPeerAddress] = func(status StatusPacket) {
		select {
		case peerAckChan <- status:
		default:
		}
	}
	gossipPacket := GossipPacket{Rumor: &rumour}
	gossiper.writeToAddr(gossipPacket, randomPeerAddress)

	go func() {
		ticker := time.NewTicker(time.Second)
		var ackStatusPtr *StatusPacket
		ackStatusPtr = nil
		select {
		case ackStatus := <-peerAckChan:
			ackStatusPtr = &ackStatus
		case <-ticker.C:
		}

		gossiper.jobsChannel <- func() {
			close(peerAckChan)
			delete(gossiper.ackAwaitMap, randomPeerAddress)
			ticker.Stop()

			if ackStatusPtr != nil {
				nextMessage := gossiper.getNextMsgToSend((*ackStatusPtr).Want)
				if nextMessage.Rumor != nil {
					gossiper.rumorMonger(*nextMessage.Rumor, fromPeerAddress, randomPeerAddress, false)
					return
				} else if nextMessage.Status != nil {
					gossiper.writeToAddr(nextMessage, randomPeerAddress)
					return
				} else {
					printInSyncLog(randomPeerAddress)
				}
			}

			randomValue := gossiper.randGen.Int()
			i := randomValue % 2

			if i == 0 {
				if gossiper.debug {
					fmt.Println("__________Flipped coin but got Tails :(")
				}
				return
			} else {
				if gossiper.debug {
					fmt.Println("__________Flipped coin and got Heads ^_^")
				}
				gossiper.rumorMonger(rumour, randomPeerAddress, "", true)
			}

		}
	}()
}

func (gossiper *Gossiper) generateStatusPacket() (statusPacket StatusPacket) {
	var wantList []PeerStatus
	wantList = []PeerStatus{}
	for peer, messages := range gossiper.messagesMap {
		nextId := getNextWantId(messages)
		wantList = append(wantList, PeerStatus{Identifier: peer, NextId: nextId})
	}

	statusPacket = StatusPacket{Want: wantList}
	return
}

func (gossiper *Gossiper) storeMessage(message GenericMessage) {
	messageList := gossiper.messagesMap[message.Origin]
	messageList = append(messageList, message)
	gossiper.messagesMap[message.Origin] = messageList
}

func (gossiper *Gossiper) isRumorNews(rumour *RumorMessage) (news bool) {
	news = true
	originMessages := gossiper.messagesMap[rumour.Origin]
	nextWantId := getNextWantId(originMessages)

	// We don't want anything other than the next expected rumor
	if rumour.ID != nextWantId {
		news = false
	}
	return
}

func (gossiper *Gossiper) listenUi(wg *sync.WaitGroup) {
	defer wg.Done()

	messageHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done := make(chan bool)
		gossiper.jobsChannel <- func() {
			switch r.Method {
			case http.MethodGet:
				{
					mapJson, err := json.Marshal(gossiper.messagesMap)
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
						printClientMessageLog(packet.Rumor.Text)
					} else if packet.Simple != nil {
						printClientMessageLog(packet.Simple.Contents)
					}
					gossiper.printPeers()

					if packet.Rumor != nil {
						messages := gossiper.messagesMap[gossiper.Name]
						nextId := getNextWantId(messages)

						packet.Rumor.ID = nextId
						packet.Rumor.Origin = gossiper.Name

						message := getGenericMessageFromRumor(*packet.Rumor)
						gossiper.storeMessage(message)
						gossiper.rumorMonger(*packet.Rumor, "", "", false)
					} else if packet.Private != nil {
						packet.Private.Origin = gossiper.Name
						packet.Private.ID = 0
						packet.Private.HopLimit = 10

						nextHop := gossiper.routingTable[packet.Private.Destination]
						gossiper.writeToAddr(packet, nextHop)
					} else { // SimpleMessage
						gossiper.broadcast(packet)
					}
				}
			default:
				http.Error(w, "Unsupported request method.", 405)
			}
			done <- true
		}
		<-done
	})

	nodeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done := make(chan bool)
		gossiper.jobsChannel <- func() {
			switch r.Method {
			case http.MethodGet:
				{
					listJson, err := json.Marshal(gossiper.Peers)
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
					gossiper.jobsChannel <- func() {
						gossiper.Peers = append(gossiper.Peers, string(node[:]))
					}
				}
			default:
				http.Error(w, "Unsupported request method.", 405)
			}
			done <- true
		}
		<-done
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

	originHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done := make(chan bool)
		gossiper.jobsChannel <- func() {
			switch r.Method {
			case http.MethodGet:
				{
					origins := []string{}
					for origin := range gossiper.routingTable {
						origins = append(origins, origin)
					}
					listJson, err := json.Marshal(origins)
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
			done <- true
		}
		<-done

	})

	fileHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done := make(chan bool)
		gossiper.jobsChannel <- func() {
			switch r.Method {
			case http.MethodPost:
				{
					fileName, err := ioutil.ReadAll(r.Body)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					gossiper.jobsChannel <- func() {
						gossiper.indexFile(string(fileName))
					}
				}
			case http.MethodGet:
				{
					params := r.URL.Query()
					gossiper.jobsChannel <- func() {
						gossiper.requestFile(params["metaHash"][0], params["destination"][0], params["fileName"][0])
					}
				}
			default:
				http.Error(w, "Unsupported request method.", 405)
			}
			done <- true
		}
		<-done

	})
	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir("./client/static"))

	mux.Handle("/", fileServer)
	mux.Handle("/message", messageHandler)
	mux.Handle("/node", nodeHandler)
	mux.Handle("/id", idHandler)
	mux.Handle("/origin", originHandler)
	mux.Handle("/file", fileHandler)

	if gossiper.debug {
		fmt.Printf("UI Server starting on :%s\n", gossiper.uiPort)
	}
	err := http.ListenAndServe(fmt.Sprintf(":%s", gossiper.uiPort), mux)

	if err != nil {
		panic(err)
	}
}

func (gossiper *Gossiper) indexFile(fileName string) (success bool) {
	chunkSize := 8 * 1024
	file, openErr := os.Open(fmt.Sprintf("./_SharedFiles/%s", fileName))
	defer file.Close()

	if openErr != nil {
		return false
	}

	chunks := [][]byte{}
	size := uint32(0)
	for {
		buffer := make([]byte, chunkSize)
		var readErr error
		readCount, readErr := file.Read(buffer)
		if readErr != nil {
			if readErr.Error() == "EOF" {
				break
			} else {
				return false
			}
		}
		size += uint32(readCount)
		chunks = append(chunks, buffer[0:readCount])
	}

	metaFile := []byte{}
	for _, chunk := range chunks {
		hash := sha256.Sum256(chunk)
		gossiper.fileContentMap[hex.EncodeToString(hash[:])] = chunk
		metaFile = append(metaFile, hash[:]...)
	}
	metaHash := sha256.Sum256(metaFile)
	metaHashHex := hex.EncodeToString(metaHash[:])
	fmt.Println("meta", metaHashHex)
	gossiper.fileContentMap[metaHashHex] = metaFile

	gossiper.fileList = append(gossiper.fileList, FileIndex{
		Name:     fileName,
		Size:     size,
		MetaFile: metaFile,
		MetaHash: metaHash,
	})

	return true
}

func (gossiper *Gossiper) getNextMsgToSend(peerWants []PeerStatus) (gossip GossipPacket) {
	gossip = GossipPacket{}

	// Check for any news I have
outer:
	for origin := range gossiper.messagesMap {
		if len(gossiper.messagesMap[origin]) > 0 {
			for _, peerStatus := range peerWants {
				if peerStatus.Identifier == origin {
					peerWant := peerStatus.NextId
					onlyGossips := getOnlyGossips(gossiper.messagesMap[origin])
					for _, message := range onlyGossips {
						if message.ID >= peerWant {
							nextRumor := getRumorFromGenericMessage(message)
							gossip = GossipPacket{Rumor: &nextRumor}
							return
						}
					}
					continue outer
				}
			}
			onlyGossips := getOnlyGossips(gossiper.messagesMap[origin])
			nextRumor := getRumorFromGenericMessage(onlyGossips[0])
			gossip = GossipPacket{Rumor: &nextRumor}
			return
		}
	}

	// Execution coming here means I have no news to send. So check if peer has news

	for _, peerStatus := range peerWants {
		if peerStatus.NextId == 1 {
			continue
		}

		peerId := peerStatus.Identifier
		if getNextWantId(gossiper.messagesMap[peerId]) < peerStatus.NextId {
			statusPacket := gossiper.generateStatusPacket()
			gossip = GossipPacket{Status: &statusPacket}
			return
		}

	}

	// Execution coming here means neither of us have news.
	return
}

func (gossiper *Gossiper) writeToAddr(packet GossipPacket, address string) {
	if packet.Rumor != nil {
		printMongeringWithLog(address)
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
	defer gossiper.entropyTicker.Stop()

	for range gossiper.entropyTicker.C {
		gossiper.jobsChannel <- func() {
			if len(gossiper.Peers) > 0 {
				randomPeerIdx := gossiper.randGen.Intn(len(gossiper.Peers))
				statusPacket := gossiper.generateStatusPacket()
				gossipPacket := GossipPacket{Status: &statusPacket}
				gossiper.writeToAddr(gossipPacket, gossiper.Peers[randomPeerIdx])
			}
		}
	}
}

func (gossiper *Gossiper) announceRoutes(wg *sync.WaitGroup) {
	defer wg.Done()
	defer gossiper.routingTicker.Stop()

	for ; true; <-gossiper.routingTicker.C {
		gossiper.jobsChannel <- func() {
			packet := GossipPacket{Rumor: &RumorMessage{Text: ""}}
			messages := gossiper.messagesMap[gossiper.Name]
			nextId := getNextWantId(messages)

			packet.Rumor.ID = nextId
			packet.Rumor.Origin = gossiper.Name

			message := getGenericMessageFromRumor(*packet.Rumor)
			gossiper.storeMessage(message)
			gossiper.rumorMonger(*packet.Rumor, "", "", false)
		}
	}
}

func (gossiper *Gossiper) executeJobs(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		job := <-gossiper.jobsChannel
		job()
	}
}
