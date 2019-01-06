package main

import (
	"bytes"
	"crypto/rsa"
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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Gossiper struct {
	jobsChannel              chan func()
	gossipAddress            *net.UDPAddr
	gossipConn               *net.UDPConn
	uiPort                   string
	Name                     string
	simple                   bool
	Peers                    []string
	ackAwaitMap              map[string]func(status StatusPacket)
	fileAwaitMap             map[string]func(reply DataReply)
	searchAwaitMap           map[string]func(reply SearchReply)
	messagesMap              map[string][]GenericMessage
	routingTable             map[string]string
	randGen                  *rand.Rand
	debug                    bool
	entropyTicker            *time.Ticker
	routingTicker            *time.Ticker
	fileList                 []FileIndex
	fileContentMap           map[string][]byte
	currentDownloads         map[string][]byte
	lastSearchRequest        map[string]int64
	searchResults            map[string]map[uint64][]string
	fileMetaMap              map[string][]byte
	blockChain               []Block
	publishedTxs             []TxPublish
	forks                    [][]Block
	blockChainEventLoop      chan func()
	txChannel                chan TxPublish
	strayBlocks              []Block
	neighbours               []Peer
	pastryRoutingTable       [8][4]Peer
	upperLeafSet             []Peer
	lowerLeafSet             []Peer
	fileReplicateAwaitMap    map[string]func(ack FileReplicateAck)
	fileReplicatedTargetsMap map[string][]string
	fileStreamableSrcMap     map[string]string
	key                      *rsa.PrivateKey
	keyMap                   map[string]*rsa.PublicKey
	blockchainBootstrap      bool
}

type FileIndex struct {
	Name          string
	Size          uint32
	MetaFile      []byte
	MetaHash      []byte
	StreamableSrc string
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

		packetBytes := make([]byte, 65536) // 64KB
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
				printPeersLog(gossiper.Peers)

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

func (gossiper *Gossiper) requestFileChunk(metaHashHex string, metaFile []byte, fromIndex int, fileName string, dest string, done chan bool) {
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

	destination := dest
	if destination == "" {
		chunkIndex := uint64(fromIndex / 32)

		searchResults, found := gossiper.searchResults[metaHashHex]
		if found {
			somePeersHavingChunk, exists := searchResults[chunkIndex+1]
			if exists {
				chosenPeerIdx := gossiper.randGen.Intn(len(somePeersHavingChunk))
				destination = somePeersHavingChunk[chosenPeerIdx]
			} else {
				if gossiper.debug {
					fmt.Printf("__________Search result peer list for the chunk %d of file %s not available\n", chunkIndex, fileName)
				}
				done <- false
				return
			}
		} else {
			if gossiper.debug {
				fmt.Printf("__________Search results for the file %s not available\n", fileName)
			}
			done <- false
			return
		}

		if gossiper.debug {
			fmt.Printf("__________Requesting chunk %d from peer %s\n", chunkIndex, destination)
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
					if gossiper.debug {
						fmt.Println("__________Received a reply for file chunk request")
					}
					gossiper.fileContentMap[hashChunkHex] = replyPtr.Data
					downloadedChunks := gossiper.currentDownloads[metaHashHex] // Should exist for sure
					gossiper.currentDownloads[metaHashHex] = append(downloadedChunks, replyPtr.Data...)
					gossiper.requestFileChunk(metaHashHex, metaFile, endIndex, fileName, dest, done)
					return
				}
				// keep retrying if I didn't get what I want
				gossiper.requestFileChunk(metaHashHex, metaFile, fromIndex, fileName, dest, done)
			}
		}()
	}
}

// TODO: reconsider if storing sie of file is really necessary
func (gossiper *Gossiper) initiateFileDownload(metaHashHex string, fileName string, dest string, done chan bool) {
	metaHash, err := hex.DecodeString(metaHashHex)
	if err != nil {
		if gossiper.debug {
			fmt.Printf("__________Failed to decode hex string %s to []byte\n", metaHashHex)
		}
		done <- false
		return
	}

	dataReplyChan := make(chan DataReply)
	gossiper.fileAwaitMap[metaHashHex] = func(reply DataReply) {
		select {
		case dataReplyChan <- reply:
		default:
		}
	}

	destination := dest
	if destination == "" {
		searchResults, found := gossiper.searchResults[metaHashHex]
		if found {
			somePeersHavingFile, exists := searchResults[1]
			if exists {
				chosenPeerIdx := gossiper.randGen.Intn(len(somePeersHavingFile))
				destination = somePeersHavingFile[chosenPeerIdx]
			} else {
				if gossiper.debug {
					fmt.Printf("__________Search result peer list for the chunk 0 of file %s not available\n", fileName)
				}
				done <- false
				return
			}
		} else {
			if gossiper.debug {
				fmt.Printf("__________Search results for the file %s not available\n", fileName)
			}
			done <- false
			return
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
					// To be able to be served to another peer
					gossiper.fileContentMap[metaHashHex] = replyPtr.Data
					transcodedPath := ""
					if replyPtr.StreamableSrc != "" {
						transcodedPath = fmt.Sprintf("http://%s:%s/streaming/%s.mp3", gossiper.gossipAddress.IP.String(), gossiper.uiPort, strings.TrimSuffix(fileName, filepath.Ext(fileName)))
					}
					gossiper.fileList = append(gossiper.fileList, FileIndex{
						Name:          fileName,
						Size:          0, // TODO set the file size once the download is done
						MetaFile:      replyPtr.Data,
						MetaHash:      metaHash,
						StreamableSrc: transcodedPath,
					})
					gossiper.requestFileChunk(metaHashHex, replyPtr.Data, 0, fileName, dest, done)
					return
				}
				// keep retrying if I didn't get what I want
				gossiper.initiateFileDownload(metaHashHex, fileName, dest, done)
			}
		}()
	}
}

func (gossiper *Gossiper) requestFile(metaHashHex string, destination string, saveAs string, status chan bool) {
	gossiper.currentDownloads[metaHashHex] = []byte{}
	done := make(chan bool)

	if gossiper.debug {
		fmt.Printf("__________Initiating file download with metahash %s saveAs %s dest %s\n", metaHashHex, saveAs, destination)
	}
	gossiper.initiateFileDownload(metaHashHex, saveAs, destination, done)

	go func() {
		success := <-done
		if success {
			content := gossiper.currentDownloads[metaHashHex]
			writeToFile(content, saveAs)
			transcodedPath := gossiper.transcodeStreamableFile(saveAs)
			if transcodedPath != "" {
				gossiper.fileStreamableSrcMap[metaHashHex] = transcodedPath
			}
			printFileReconstructLog(saveAs)
			delete(gossiper.currentDownloads, metaHashHex)
		} else {
			fmt.Println("Failed to download file.")
			delete(gossiper.currentDownloads, metaHashHex)
		}
		status <- success

	}()
}

func (gossiper *Gossiper) handleGossip(packet GossipPacket, relayPeer string) {
	if packet.Rumor != nil {
		rumorHandler(packet, relayPeer, gossiper)
	} else if packet.Status != nil {
		statusHandler(packet, relayPeer, gossiper)
	} else if packet.Private != nil {
		privateHandler(packet, gossiper)
	} else if packet.EncPrivate != nil {
		encPrivateHandler(packet, gossiper)
	} else if packet.DataRequest != nil {
		dataRequestHandler(packet, gossiper)
	} else if packet.DataReply != nil {
		dataReplyHandler(packet, gossiper)
	} else if packet.SearchRequest != nil {
		searchRequestHandler(packet, gossiper)
	} else if packet.SearchReply != nil {
		searchReplyHandler(packet, gossiper)
	} else if packet.TxPublish != nil {
		txPublishHandler(packet, gossiper)
	} else if packet.BlockPublish != nil {
		blockPublishHandler(packet, gossiper)
	} else if packet.NeighbourNotification != nil {
		neighbourNotificationHandler(packet, gossiper)
	} else if packet.NotificationResponse != nil {
		notificationResponseHandler(packet, gossiper)
	} else if packet.FilePullRequest != nil {
		filePullRequestHandler(packet, gossiper)
	} else if packet.FileReplicateAck != nil {
		fileReplicateAckHandler(packet, gossiper)
	} else if packet.BlockChainReply != nil {
		blockchainReplyHandler(packet, gossiper)
	} else if packet.BlockChainRequest != nil {
		blockchainRequestHandler(packet, gossiper)
	} else {
		if gossiper.debug {
			fmt.Println("__________Unexpected gossip packet type.")
		}
	}
}

func (gossiper *Gossiper) redistributeSearchRequest(request *SearchRequest) {
	budget := request.Budget
	peerCount := uint64(len(gossiper.Peers))

	selectedPeers := []string{}
	chosenBudgets := []uint64{}
	if budget <= peerCount {
	outer:
		for uint64(len(selectedPeers)) != budget {
			index := gossiper.randGen.Intn(int(peerCount))
			candidatePeer := gossiper.Peers[index]
			for _, peer := range selectedPeers {
				if candidatePeer == peer {
					continue outer
				}
			}

			selectedPeers = append(selectedPeers, candidatePeer)
			chosenBudgets = append(chosenBudgets, 1)
		}
	} else {
		selectedPeers = gossiper.Peers
		chosenBudgets = make([]uint64, peerCount)

		dividend := budget / peerCount
		remainder := budget % peerCount

		for index := range selectedPeers {
			if uint64(index) < remainder {
				chosenBudgets[index] = uint64(dividend + 1)
			} else {
				chosenBudgets[index] = uint64(dividend)
			}
		}
	}

	if gossiper.debug {
		fmt.Printf("__________Selected peers   %v\n", selectedPeers)
		fmt.Printf("__________Selected budgets %v\n", chosenBudgets)
	}

	for index, peer := range selectedPeers {
		request.Budget = chosenBudgets[index]
		gossiper.writeToAddr(GossipPacket{SearchRequest: request}, peer)
	}
}

func (gossiper *Gossiper) forwardPrivateMessage(private *PrivateMessage) {
	address, found := gossiper.routingTable[private.Destination]
	if found {
		packet := GossipPacket{Private: private}
		gossiper.writeToAddr(packet, address)
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward private message. Next hop for %s not found.\n", private.Destination)
		}
	}

}

func (gossiper *Gossiper) forwardDataRequest(request *DataRequest) (success bool) {
	address, found := gossiper.routingTable[request.Destination]
	if found {
		packet := GossipPacket{DataRequest: request}
		gossiper.writeToAddr(packet, address)

		if gossiper.debug {
			fmt.Printf("__________Forwarded data request with origin %s dest %s hop-limit %d to %s\n", request.Origin, request.Destination, request.HopLimit, address)
		}
		return true
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward data request. Next hop for %s not found.\n", request.Destination)
		}
		return false
	}

}

func (gossiper *Gossiper) forwardDataReply(reply *DataReply) (success bool) {
	address, found := gossiper.routingTable[reply.Destination]
	if found {
		packet := GossipPacket{DataReply: reply}
		gossiper.writeToAddr(packet, address)

		if gossiper.debug {
			fmt.Printf("__________Forwarded data reply with origin %s dest %s hop-limit %d to %s\n", reply.Origin, reply.Destination, reply.HopLimit, address)
		}
		return true
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward data reply. Next hop for %s not found.\n", reply.Destination)
		}
		return false
	}

}

func (gossiper *Gossiper) forwardSearchReply(reply *SearchReply) (success bool) {
	address, found := gossiper.routingTable[reply.Destination]
	if found {
		packet := GossipPacket{SearchReply: reply}
		gossiper.writeToAddr(packet, address)

		if gossiper.debug {
			fmt.Printf("__________Forwarded search reply with origin %s dest %s hop-limit %d to %s\n", reply.Origin, reply.Destination, reply.HopLimit, address)
		}
		return true
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward search reply. Next hop for %s not found.\n", reply.Destination)
		}
		return false
	}

}

func (gossiper *Gossiper) forwardNeighbourNotification(notification *NeighbourNotification, attempt int) (success bool) {
	address, found := gossiper.routingTable[notification.Destination]
	if found {
		packet := GossipPacket{NeighbourNotification: notification}
		gossiper.writeToAddr(packet, address)

		if gossiper.debug {
			fmt.Printf("__________Forwarded notification with origin %s dest %s to %s\n", notification.Origin, notification.Destination, address)
		}
		return true
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward neighbour notification. Next hop for %s not found.\n", notification.Destination)
		}
		if attempt < 6 {
			go func() {
				<-time.After(5 * time.Second)
				if gossiper.debug {
					fmt.Println("__________Retrying forwarding neighbour notification. Checking for next hop", notification.Destination)
				}
				gossiper.jobsChannel <- func() {
					gossiper.forwardNeighbourNotification(notification, attempt+1)
				}
			}()
		} else {
			if gossiper.debug {
				fmt.Printf("__________Retried forwarding neighbour notification 5 times. Next hop for %s not found. Giving up.\n ", notification.Destination)
			}
		}
		return false
	}

}

func (gossiper *Gossiper) forwardNotificationResponse(response *NotificationResponse, attempt int) (success bool) {
	address, found := gossiper.routingTable[response.Destination]
	if found {
		packet := GossipPacket{NotificationResponse: response}

		gossiper.writeToAddr(packet, address)

		if gossiper.debug {
			fmt.Printf("__________Forwarded notification response %v with dest %s to %s\n", response, response.Destination, address)
		}
		return true
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward notification respons. Next hop for %s not found.\n", response.Destination)
		}
		if attempt < 6 {
			go func() {
				<-time.After(5 * time.Second)
				if gossiper.debug {
					fmt.Println("__________Retrying forwarding neighbour response. Checking for next hop", response.Destination)
				}
				gossiper.jobsChannel <- func() {
					gossiper.forwardNotificationResponse(response, attempt+1)
				}
			}()
		} else {
			if gossiper.debug {
				fmt.Printf("__________Retried forwarding neighbour response 5 times. Next hop for %s not found. Giving up.\n ", response.Destination)
			}
		}

		return false
	}

}

func (gossiper *Gossiper) forwardFilePullRequest(pullRequest *FilePullRequest, attempt int) (success bool) {
	address, found := gossiper.routingTable[pullRequest.Destination]
	if found {
		packet := GossipPacket{FilePullRequest: pullRequest}
		gossiper.writeToAddr(packet, address)

		if gossiper.debug {
			fmt.Printf("__________Forwarded pull request with hash %x dest %s to %s\n", pullRequest.HashValue, pullRequest.Destination, address)
		}
		return true
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward pull request. Next hop for %s not found.\n", pullRequest.Destination)
		}
		if attempt < 2 {
			go func() {
				<-time.After(5 * time.Second)
				if gossiper.debug {
					fmt.Println("__________Retrying forwarding pull request. Checking for next hop", pullRequest.Destination)
				}
				gossiper.jobsChannel <- func() {
					gossiper.forwardFilePullRequest(pullRequest, attempt+1)
				}
			}()
		} else {
			if gossiper.debug {
				fmt.Printf("__________Retried PR 2 times. Next hop for %s not found. Giving up.\n ", pullRequest.Destination)
			}
		}
		return false
	}

}

func (gossiper *Gossiper) forwardFileReplicateAck(ack *FileReplicateAck, attempt int) (success bool) {
	address, found := gossiper.routingTable[ack.Destination]
	if found {
		packet := GossipPacket{FileReplicateAck: ack}
		gossiper.writeToAddr(packet, address)

		if gossiper.debug {
			fmt.Printf("__________Forwarded file replicate ack with hash %x dest %s to %s\n", ack.HashValue, ack.Destination, address)
		}
		return true
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward file replicate ack. Next hop for %s not found.\n", ack.Destination)
		}
		if attempt < 5 {
			go func() {
				<-time.After(5 * time.Second)
				if gossiper.debug {
					fmt.Println("__________Retrying forwarding replicate ack. Checking for next hop", ack.Destination)
				}
				gossiper.jobsChannel <- func() {
					gossiper.forwardFileReplicateAck(ack, attempt+1)
				}
			}()
		} else {
			if gossiper.debug {
				fmt.Printf("__________Retried frowarding replicate ack 5 times. Next hop for %s not found. Giving up.\n ", ack.Destination)
			}
		}
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
					} else if packet.Private != nil {
						printClientMessageLog(packet.Private.Text)
					} else if packet.Simple != nil {
						printClientMessageLog(packet.Simple.Contents)
					}
					printPeersLog(gossiper.Peers)

					if packet.Rumor != nil {
						messages := gossiper.messagesMap[gossiper.Name]
						nextId := getNextWantId(messages)

						packet.Rumor.ID = nextId
						packet.Rumor.Origin = gossiper.Name

						message := getGenericMessageFromRumor(*packet.Rumor)
						gossiper.storeMessage(message)
						if len(gossiper.Peers) > 0 {
							gossiper.rumorMonger(*packet.Rumor, "", "", false)
						}
					} else if packet.Private != nil {
						packet.Private.Origin = gossiper.Name
						packet.Private.ID = 0
						packet.Private.HopLimit = 10

						nextHop, found := gossiper.routingTable[packet.Private.Destination]
						if found {
							gossiper.writeToAddr(packet, nextHop)
						} else {
							if gossiper.debug {
								fmt.Printf("Could not forward client private message %s. "+
									"Next hop for %s not found\n", packet.Private.Text, packet.Private.Destination)
							}
						}
					} else if packet.EncPrivate != nil {
						dest := packet.EncPrivate.Destination

						if destKey, ok := gossiper.keyMap[dest]; ok {
							packet.EncPrivate, err = NewEncPrivateMsg(gossiper.Name, packet.EncPrivate.Temp, dest, gossiper.key, destKey)
							if err != nil {
								if gossiper.debug {
									fmt.Println("ERR", err)
								}
							}

							nextHop, found := gossiper.routingTable[packet.EncPrivate.Destination]
							if found {
								gossiper.writeToAddr(packet, nextHop)
								fmt.Println("SENDING ENC PRIVATE")
							} else {
								fmt.Printf("Could not forward client private message %s. "+
									"Next hop for %s not found\n", packet.Private.Text, packet.Private.Destination)
								if gossiper.debug {
									fmt.Printf("Could not forward client private message %s. "+
										"Next hop for %s not found\n", packet.Private.Text, packet.Private.Destination)
								}
							}

						} else {
							if gossiper.debug {
								fmt.Printf("Could not find destination %s public key.", dest)
							}
						}

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
		statusChan := make(chan bool)
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
						printFileIndexingLog(string(fileName))
						status := gossiper.indexFile(string(fileName))
						statusChan <- status
					}
				}
			case http.MethodGet:
				{
					params := r.URL.Query()
					destinationParam := params["destination"]
					destination := ""
					if len(destinationParam) > 0 {
						destination = destinationParam[0]
					}

					gossiper.jobsChannel <- func() {
						gossiper.requestFile(params["metaHash"][0], destination, params["fileName"][0], statusChan)
					}
				}
			default:
				http.Error(w, "Unsupported request method.", 405)
				statusChan <- false
			}
		}
		status := <-statusChan
		w.Write([]byte(strconv.FormatBool(status)))
	})

	searchHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assume it is very unlikely to have 20 total matches simultaneously
		statusChan := make(chan string, 20)
		gossiper.jobsChannel <- func() {
			switch r.Method {
			case http.MethodGet:
				{
					params := r.URL.Query()
					gossiper.jobsChannel <- func() {
						budgetParam := params["budget"]
						budget := ""

						if len(budgetParam) > 0 {
							budget = budgetParam[0]
						}

						gossiper.searchFile(params["keywords"][0], budget, statusChan)
					}
				}
			default:
				http.Error(w, "Unsupported request method.", 405)
				statusChan <- ""
			}
		}
		for {
			file := <-statusChan
			if file != "" {
				flusher, ok := w.(http.Flusher)
				if !ok {
					fmt.Println("Error. expected http.ResponseWriter to be an http.Flusher")
				}
				w.Header().Set("X-Content-Type-Options", "nosniff")
				fmt.Fprintln(w, file)
				flusher.Flush()
			} else {
				close(statusChan)
				break
			}
		}

	})

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir("./client/static"))

	mux.Handle("/streaming/", corsHandler(http.StripPrefix("/streaming/", http.FileServer(http.Dir("./_SharedFiles/Streaming/")))))
	mux.Handle("/", corsHandler(fileServer))
	mux.Handle("/message", messageHandler)
	mux.Handle("/node", nodeHandler)
	mux.Handle("/id", idHandler)
	mux.Handle("/origin", originHandler)
	mux.Handle("/file", fileHandler)
	mux.Handle("/search", searchHandler)

	if gossiper.debug {
		fmt.Printf("UI Server starting on :%s\n", gossiper.uiPort)
	}
	err := http.ListenAndServe(fmt.Sprintf(":%s", gossiper.uiPort), mux)

	if err != nil {
		panic(err)
	}
}

func corsHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var origin, method, headers string

		origin = r.Header.Get("Origin")

		if r.Method == "OPTIONS" {
			method = r.Header.Get("Access-Control-Request-Method")
			headers = r.Header.Get("Access-Control-Request-Headers")

			if len(origin) == 0 || len(method) == 0 {
				msg := fmt.Sprintf("%d %s: missing required CORS headers",
					http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
		}

		if len(origin) > 0 {
			w.Header().Add("Vary", "Origin")
			w.Header().Add("Access-Control-Allow-Origin", origin)
			w.Header().Add("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Access-Control-Expose-Headers", "Accept-Ranges,Content-Range,Content-Type,Authorization,Content-Length,Vary")
		}

		if len(method) > 0 {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Access-Control-Allow-Methods", method)
		}

		if len(headers) > 0 {
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.Header().Add("Access-Control-Allow-Headers", headers)
		}

		if r.Method != "OPTIONS" {
			handler.ServeHTTP(w, r)
		}
	})
}

func isTotalMatch(fileChunks map[uint64][]string, expectedCount uint64) (isMatch bool) {
	isMatch = false

	availableChunks := []uint64{}

	for key := range fileChunks {
		availableChunks = append(availableChunks, key)
	}

	if uint64(len(availableChunks)) == expectedCount {
		isMatch = true
	}

	return
}

func (gossiper *Gossiper) searchFile(keywords string, budgetStr string, status chan string) {
	if gossiper.debug {
		fmt.Println("searching files for keywords ", keywords)
	}

	matchThreshold := 2
	budgetThreshold := uint64(32)
	guiAwaitTimedOut := false
	guiTimeoutSecs := 20

	go func() {
		ticker := time.NewTicker(time.Duration(guiTimeoutSecs) * time.Second)
		<-ticker.C
		guiAwaitTimedOut = true
		select {
		case status <- "":
		default:
		}
	}()

	matchThresholdMet := make(chan bool)

	keywordsArr := strings.Split(keywords, ",")

	searchReplyChan := make(chan SearchReply, len(keywordsArr)*5)

	gossiper.searchAwaitMap[keywords] = func(reply SearchReply) {
		select {
		case searchReplyChan <- reply:
		default:
		}
	}
	totalMatches := []string{}

	go func() {
		for {
			reply := <-searchReplyChan

			if gossiper.debug {
				fmt.Println("__________SearchReply handler received reply from", reply.Origin)
			}
			gossiper.jobsChannel <- func() {
				results := reply.Results

			outer:
				for _, result := range results {
					printSearchResultLog(result.FileName, reply.Origin, result.MetafileHash, result.ChunkMap)
					metaHash := hex.EncodeToString(result.MetafileHash)

				inner:
					for _, chunkIdx := range result.ChunkMap {
						_, found := gossiper.searchResults[metaHash]

						if !found {
							gossiper.searchResults[metaHash] = make(map[uint64][]string)
						}

						list, exists := gossiper.searchResults[metaHash][chunkIdx]

						if !exists {
							peers := []string{reply.Origin}
							gossiper.searchResults[metaHash][chunkIdx] = peers
						} else {
							for _, peer := range list {
								if peer == reply.Origin {
									continue inner
								}
							}

							gossiper.searchResults[metaHash][chunkIdx] = append(list, reply.Origin)
						}
					}

					for _, match := range totalMatches {
						if match == result.FileName {
							continue outer
						}
					}

					if isTotalMatch(gossiper.searchResults[metaHash], result.ChunkCount) {
						totalMatches = append(totalMatches, result.FileName)
						if len(totalMatches) == matchThreshold {
							select {
							case matchThresholdMet <- true:
							default:
							}
							printSearchFinishedLog()
						}

						if gossiper.debug {
							fmt.Printf("__________Result map for %s - %v\n", result.FileName, gossiper.searchResults[metaHash])
						}
						if !guiAwaitTimedOut {
							returnString := fmt.Sprintf("%s,%s,%s", result.FileName, metaHash, result.StreamableSrc)

							select {
							case status <- returnString:
							default:
							}
						}
					}
				}
			}
		}
	}()

	if budgetStr == "" {
		budget := uint64(2)

		ticker := time.NewTicker(time.Second)
		go func() {
			for ; true; <-ticker.C {
				select {
				case <-matchThresholdMet:
					break
				default:
				}

				if budget <= budgetThreshold {
					gossiper.jobsChannel <- func() {
						request := SearchRequest{Origin: gossiper.Name, Budget: budget, Keywords: keywordsArr}
						gossiper.redistributeSearchRequest(&request)
						budget *= 2
					}
				} else {
					ticker.Stop()
					break
				}
			}
		}()
	} else {
		budget, err := strconv.ParseUint(budgetStr, 0, 64)
		if err != nil {
			if gossiper.debug {
				fmt.Printf("__________Failed to parse given budget %s into uint64\n", budgetStr)
			}
			status <- ""
			return
		}
		request := SearchRequest{Origin: gossiper.Name, Budget: budget, Keywords: keywordsArr}

		gossiper.jobsChannel <- func() {
			gossiper.redistributeSearchRequest(&request)
		}
	}
}

func (gossiper *Gossiper) indexFile(fileName string) (success bool) {
	chunkSize := 8 * 1024
	file, openErr := os.Open(fmt.Sprintf("./_SharedFiles/%s", fileName))
	defer file.Close()

	if openErr != nil {
		if gossiper.debug {
			fmt.Println("__________Failed to open file while indexing", fileName)
		}
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
	gossiper.fileContentMap[metaHashHex] = metaFile
	transcodedPath := gossiper.transcodeStreamableFile(fileName)
	if transcodedPath != "" {
		gossiper.fileStreamableSrcMap[metaHashHex] = transcodedPath
	}

	gossiper.fileList = append(gossiper.fileList, FileIndex{
		Name:          fileName,
		Size:          size,
		MetaFile:      metaFile,
		MetaHash:      metaHash[:],
		StreamableSrc: transcodedPath,
	})

	printFileIndexingCompletedLog(fileName, metaHashHex)

	gossiper.sendPullRequests(metaHash[:], fileName, 5)
	gossiper.txPublishFile(fileName, int64(size), metaHash[:], transcodedPath)

	return true
}

func (gossiper *Gossiper) sendPullRequests(metaHash []byte, fileName string, replicationCount int) {
	hashChunkHex := hex.EncodeToString(metaHash)
	if replicationCount == 0 {
		printReplicationCompleteLog(gossiper.fileReplicatedTargetsMap[hashChunkHex])
		return
	}

	ackChan := make(chan FileReplicateAck)

	gossiper.fileReplicateAwaitMap[hashChunkHex] = func(ack FileReplicateAck) {
		select {
		case ackChan <- ack:
		default:
		}
	}

	randomId := generateResourceId()
	closestId := getNumericallyClosestId(randomId, gossiper)
	pullRequest := FilePullRequest{Origin: gossiper.Name, Destination: closestId, HashValue: metaHash, FileName: fileName, FileId: randomId}
	if gossiper.debug {
		fmt.Printf("__________Sending pull request for file ID %s to %s\n", randomId, closestId)
	}
	gossiper.forwardFilePullRequest(&pullRequest, 0)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		var ackPtr *FileReplicateAck
		ackPtr = nil
		select {
		case ack := <-ackChan:
			ackPtr = &ack
		case <-ticker.C:
		}

		gossiper.jobsChannel <- func() {
			close(ackChan)
			delete(gossiper.fileReplicateAwaitMap, hashChunkHex)
			ticker.Stop()

			if ackPtr != nil {
				if gossiper.debug {
					fmt.Println("__________Received a valid ack", *ackPtr)
				}

				//check if the target has recurred
				targets, found := gossiper.fileReplicatedTargetsMap[hashChunkHex]
				if found {
					for _, target := range targets {
						if target == ackPtr.Origin {
							gossiper.sendPullRequests(metaHash, fileName, replicationCount)
							return
						}
					}
					gossiper.fileReplicatedTargetsMap[hashChunkHex] = append(gossiper.fileReplicatedTargetsMap[hashChunkHex], ackPtr.Origin)
				} else {
					ts := []string{ackPtr.Origin}
					gossiper.fileReplicatedTargetsMap[hashChunkHex] = ts
				}

				gossiper.sendPullRequests(metaHash, fileName, replicationCount-1)
				return
			}
			gossiper.sendPullRequests(metaHash, fileName, replicationCount)
		}
	}()
}

func (gossiper *Gossiper) txPublishFile(fileName string, size int64, metaHash []byte, streamableSrc string) {
	for _, tx := range gossiper.publishedTxs {
		if tx.File.Name == fileName &&
			tx.File.Size == size &&
			bytes.Equal(tx.File.MetafileHash, metaHash) {
			return
		}
	}

	_, found := gossiper.fileMetaMap[fileName]

	if found {
		return
	}

	txPubFile := File{Name: fileName, Size: int64(size), MetafileHash: metaHash, StreamableSrc: streamableSrc}

	txPub := TxPublish{File: txPubFile, HopLimit: 10}

	gossiper.txChannel <- txPub

	gossiper.broadcast(GossipPacket{TxPublish: &txPub})
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

	_, err = gossiper.gossipConn.WriteToUDP(packetBytes, destinationAddress)
	if err != nil {
		fmt.Println("UDP ERR:", err)
	}
}

func (gossiper *Gossiper) broadcast(packet GossipPacket) {
	relayPeerAddress := ""
	if packet.Simple != nil {
		if packet.Simple.OriginalName == "" {
			packet.Simple.OriginalName = gossiper.Name
		}

		relayPeerAddress = packet.Simple.RelayPeerAddr
		packet.Simple.RelayPeerAddr = gossiper.gossipAddress.String()
	}

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

func (gossiper *Gossiper) notifyANeighbour() {
	gossiper.jobsChannel <- func() {
		if len(gossiper.Peers) == 1 && gossiper.Peers[0] == gossiper.gossipAddress.String() {
			fmt.Println("i have only one and its me")
			return
		} else {

			// Quick fix for test script giving out oneself as ones peer
			randomPeerIdx := gossiper.randGen.Intn(len(gossiper.Peers))
			for gossiper.Peers[randomPeerIdx] == gossiper.gossipAddress.String() {
				randomPeerIdx = gossiper.randGen.Intn(len(gossiper.Peers))
			}

			notification := NeighbourNotification{Origin: gossiper.Name, Address: gossiper.gossipAddress.String()}
			gossipPacket := GossipPacket{NeighbourNotification: &notification}
			fmt.Printf("Notifying %s\n", gossiper.Peers[randomPeerIdx])
			gossiper.writeToAddr(gossipPacket, gossiper.Peers[randomPeerIdx])
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
			if len(gossiper.Peers) > 0 {
				gossiper.rumorMonger(*packet.Rumor, "", "", false)
			}
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

func (gossiper *Gossiper) executeBlockChainJobs(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		event := <-gossiper.blockChainEventLoop
		event()
	}
}

func (gossiper *Gossiper) runBlockChainJobSync(job func()) {
	done := make(chan bool)
	gossiper.blockChainEventLoop <- func() {
		job()
		done <- true
	}
	<-done
}

func (gossiper *Gossiper) getFilteredPublishedTx(block Block) (filtered []TxPublish) {
	var filteredTxs []TxPublish
outer:
	for _, publishedTx := range gossiper.publishedTxs {
		for _, receivedTx := range block.Transactions {
			if ann := receivedTx.Announcement; ann != nil && publishedTx.Announcement != nil {
				if publishedTx.Announcement.Equal(ann) {
					fmt.Println("FILTERED")
					continue outer
				}
			} else if publishedTx.File.Name == receivedTx.File.Name {
				continue outer
			}
		}
		filteredTxs = append(filteredTxs, publishedTx)
	}

	return filteredTxs
}

func (gossiper *Gossiper) appendBlock(block Block) {

	gossiper.blockChain = append(gossiper.blockChain, block)
	printBlockChainLog(gossiper.blockChain)

	fmt.Println("APPENDING  BLOCK OF LEN:", len(block.Transactions))
	for _, tx := range block.Transactions {
		if ann := tx.Announcement; ann != nil {
			if key, err := DecodePublicKey(ann.Record.PubKey); err == nil && ann.Verify() {
				gossiper.keyMap[ann.Record.Owner] = key
				fmt.Println("FOUND KEY "+ann.Record.Owner+" "+hex.EncodeToString(ann.Record.PubKey), len(gossiper.keyMap), gossiper.keyMap)
			}

		} else {
			file := tx.File
			gossiper.fileMetaMap[file.Name] = file.MetafileHash
		}
	}
}

func (gossiper *Gossiper) clearIfStrayBlock(paramBlock Block) {
	var newStrayBlockList []Block
	for _, block := range gossiper.strayBlocks {
		existingBlockHash := block.Hash()
		receivedBlockHash := paramBlock.Hash()
		if !bytes.Equal(existingBlockHash[:], receivedBlockHash[:]) {
			newStrayBlockList = append(newStrayBlockList, block)
		}
	}
	gossiper.strayBlocks = newStrayBlockList
}

func (gossiper *Gossiper) processBlock(receivedBlock Block) (success bool) {
	gossiper.publishedTxs = gossiper.getFilteredPublishedTx(receivedBlock) // Since no point in mining already mined txs

	if len(gossiper.blockChain) == 0 {
		if gossiper.debug {
			fmt.Println("__________Empty block chain. So appending")
		}
		gossiper.appendBlock(receivedBlock)
		return true
	} else {
		headBlockHash := gossiper.blockChain[len(gossiper.blockChain)-1].Hash()
		receivedBlockPrevHash := receivedBlock.PrevHash
		// Is it extending the longest chain?
		if bytes.Equal(headBlockHash[:], receivedBlockPrevHash[:]) {
			if gossiper.debug {
				fmt.Println("__________All good. extending the longest chain")
			}
			gossiper.appendBlock(receivedBlock)
			gossiper.clearIfStrayBlock(receivedBlock)
			return true
		} else { // Is it extending a shorter chain?
			if gossiper.debug {
				fmt.Println("__________Checking if can extend shorter chain")
			}
			for index, fork := range gossiper.forks {
				forkHeadHash := fork[len(fork)-1].Hash()
				if bytes.Equal(forkHeadHash[:], receivedBlockPrevHash[:]) {
					if gossiper.debug {
						fmt.Println("__________Extending a shorter chain")
					}
					updatedFork := append(fork, receivedBlock)
					gossiper.forks[index] = updatedFork
					gossiper.clearIfStrayBlock(receivedBlock)

					// Is the chain now longest?
					if len(updatedFork) > len(gossiper.blockChain) {

						commonIdx := getCommonAncestorIndex(gossiper.blockChain, updatedFork)
						castOffs := gossiper.blockChain[commonIdx+1:]
						newJoiners := updatedFork[commonIdx+1:]

						printForkLongerLog(len(castOffs))

						for _, castOff := range castOffs {
							for _, tx := range castOff.Transactions {
								if ann := tx.Announcement; ann != nil {
									delete(gossiper.keyMap, ann.Record.Owner)

									tx := TxPublish{
										Announcement: ann,
									}
									gossiper.publishedTxs = append(gossiper.publishedTxs, tx)
								} else {
									delete(gossiper.fileMetaMap, tx.File.Name)
								}
							}
						}
						for _, newJoiner := range newJoiners {
							for _, tx := range newJoiner.Transactions {
								if ann := tx.Announcement; ann != nil {
									if key, err := DecodePublicKey(ann.Record.PubKey); err == nil && ann.Verify() {
										gossiper.keyMap[ann.Record.Owner] = key
										fmt.Println("FOUND KEY "+ann.Record.Owner+" "+hex.EncodeToString(ann.Record.PubKey), len(gossiper.keyMap), gossiper.keyMap)
									}
								} else {
									gossiper.fileMetaMap[tx.File.Name] = tx.File.MetafileHash
								}
							}
						}

						// Remove the fork from the forks list
						gossiper.forks[index] = gossiper.blockChain
						gossiper.blockChain = updatedFork
						printBlockChainLog(gossiper.blockChain)
					} else {
						printForkShorterLog(receivedBlock.Hash())
					}
					return true
				}
			}

			if gossiper.debug {
				fmt.Println("__________Checking if a new fork can be made from main chain")
			}
			// See if a new fork can be made from the main chain
			for index, block := range gossiper.blockChain {
				blockHash := block.Hash()
				if bytes.Equal(blockHash[:], receivedBlockPrevHash[:]) {
					fork := gossiper.blockChain[0 : index+1]
					fork = append(fork, receivedBlock)
					gossiper.forks = append(gossiper.forks, fork)
					gossiper.clearIfStrayBlock(receivedBlock)
					printForkShorterLog(receivedBlock.Hash())
					return true
				}
			}

			if gossiper.debug {
				fmt.Println("__________Checking if a new fork can be made from sub chains")
			}

			// see if a new fork can be made from the existing shorter chains
			for _, fork := range gossiper.forks {
				for index, block := range fork {
					blockHash := block.Hash()
					if bytes.Equal(blockHash[:], receivedBlockPrevHash[:]) {
						rootBlocks := fork[0 : index+1]
						newFork := append(rootBlocks, receivedBlock)
						gossiper.forks = append(gossiper.forks, newFork)
						gossiper.clearIfStrayBlock(receivedBlock)
						printForkShorterLog(receivedBlock.Hash())
						return true
					}
				}
			}

			if gossiper.debug {
				fmt.Println("__________Checking if this is a different first block")
			}
			var zeroBytes [32]byte
			if bytes.Equal(receivedBlock.PrevHash[:], zeroBytes[:]) {
				newFork := []Block{receivedBlock}
				gossiper.forks = append(gossiper.forks, newFork)
				gossiper.clearIfStrayBlock(receivedBlock)
				printForkShorterLog(receivedBlock.Hash())
				return true
			}

			// add to stray block list if not already there
			for _, block := range gossiper.strayBlocks {
				blockHash := block.Hash()
				receivedBlockHash := receivedBlock.Hash()
				if bytes.Equal(blockHash[:], receivedBlockHash[:]) {
					return false
				}
			}

			if gossiper.debug {
				fmt.Println("__________Adding block to stray blocks list")
			}

			gossiper.strayBlocks = append(gossiper.strayBlocks, receivedBlock)
			return false
		}
	}
}

func isValidBlock(block Block) (valid bool) {
	hash := block.Hash()

	for _, byteSingle := range hash[:2] {
		if byteSingle != 0 {
			return false
		}
	}

	if len(block.Transactions) == 0 {
		return false
	}

	return true
}

func (gossiper *Gossiper) processStrayBlocks() {
	var results []bool
	for _, block := range gossiper.strayBlocks {
		result := gossiper.processBlock(block)
		results = append(results, result)
	}
	if atLeastOneTrueExists(results) {
		gossiper.processStrayBlocks()
	}
}

func (gossiper *Gossiper) txChannelListener(wg *sync.WaitGroup) {
	defer wg.Done()

	<-time.After(10 * time.Second)
	//gossiper.bootstrapBlockchain = true

	isFirstMinedBlock := true

outer:
	for {
		tx := <-gossiper.txChannel
		for _, existingTx := range gossiper.publishedTxs {
			if tx.Announcement != nil && existingTx.Announcement != nil {
				if existingTx.Announcement.Equal(tx.Announcement) {
					//fmt.Println("TX CHANNEL")
					continue outer
				}
			} else if existingTx.File.Name == tx.File.Name {
				continue outer
			}
		}
		gossiper.publishedTxs = append(gossiper.publishedTxs, tx)
		if len(gossiper.publishedTxs) == 1 {
			gossiper.blockChainEventLoop <- func() {
				go func() {
					miningStartTimeMillis := time.Now().UnixNano() / 1000000
					for {
						if len(gossiper.publishedTxs) == 0 {
							break
						}

						var prevBlockHash [32]byte

						if len(gossiper.blockChain) > 0 {
							prevBlockHash = gossiper.blockChain[len(gossiper.blockChain)-1].Hash()
						}

						randBytes := make([]byte, 32)
						rand.Seed(time.Now().UnixNano())
						_, err := rand.Read(randBytes)
						if err != nil {
							fmt.Println("Failed to generate 32byte nonce")
							return
						}
						nonce := sha256.Sum256(randBytes)
						block := Block{PrevHash: prevBlockHash, Nonce: nonce, Transactions: gossiper.publishedTxs}
						if isValidBlock(block) {
							miningEndTimeMillis := time.Now().UnixNano() / 1000000
							mineTime := miningEndTimeMillis - miningStartTimeMillis + 1

							printSuccessfulMineLog(block.Hash())
							gossiper.runBlockChainJobSync(func() {
								gossiper.processBlock(block)
								gossiper.processStrayBlocks()
							})

							var ticker *time.Ticker
							if isFirstMinedBlock {
								ticker = time.NewTicker(5 * time.Second)
								if gossiper.debug {
									fmt.Println("__________Waiting to publish first mined block!")
								}
								isFirstMinedBlock = false
							} else {
								ticker = time.NewTicker(2 * time.Duration(mineTime) * time.Millisecond)
								if gossiper.debug {
									fmt.Println("__________Waiting to publish block!")
								}
							}
							<-ticker.C

							if gossiper.debug {
								fmt.Printf("__________Publishing mined block! Block hash %x\n", block.Hash())
							}
							blockPublish := BlockPublish{Block: block, HopLimit: 20}
							gossiper.broadcast(GossipPacket{BlockPublish: &blockPublish})

							if len(gossiper.publishedTxs) == 0 {
								break
							} else {
								miningStartTimeMillis = time.Now().UnixNano() / 1000000
							}
						}
					}
				}()
			}
		}

	}
}

func (gossiper *Gossiper) initializePastry() {
	if len(gossiper.Peers) > 0 {
		<-time.After(3 * time.Second)
		gossiper.notifyANeighbour()
	}
}

func (gossiper *Gossiper) transcodeStreamableFile(fileName string) string {
	path := fmt.Sprintf("./_SharedFiles/%s", fileName)
	nameWithoutExtension := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	outputPath := fmt.Sprintf("./_SharedFiles/Streaming/%s.mp3", nameWithoutExtension)
	// Audio Streaming - try to transcode to mp3 file if it is an audio file
	cmd := "ffmpeg"

	args := []string{"-y", "-i", path, "-error-resilient", "1", "-codec:a", "libmp3lame", "-b:a", "128k", outputPath}
	_, err := exec.Command(cmd, args...).Output()
	if err != nil {
		fmt.Println("File is not transcodable")
		return ""
	}
	fmt.Println("TRANSCODING SUCCEEDED")
	return fmt.Sprintf("http://%s:%s/streaming/%s.mp3", gossiper.gossipAddress.IP.String(), gossiper.uiPort, nameWithoutExtension)
}
