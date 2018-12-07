package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

func rumorHandler(packet GossipPacket, relayPeer string, gossiper *Gossiper) {
	printRumorMessageLog(*packet.Rumor, relayPeer)
	printPeersLog(gossiper.Peers)

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
}

func searchReplyHandler(packet GossipPacket, gossiper *Gossiper) {
	reply := packet.SearchReply
	if gossiper.debug {
		fmt.Printf("__________SearchReply received dest %s origin %s results %v hop-limit %d\n",
			reply.Destination, reply.Origin, reply.Results, reply.HopLimit)
	}
	if reply.Destination == gossiper.Name {
		results := reply.Results
		if len(results) > 0 {
			handlerKeys := getSearchHandlerKeys(gossiper.searchAwaitMap, results[0].FileName)

			if len(handlerKeys) > 0 {
				for _, handlerKey := range handlerKeys {
					handler, available := gossiper.searchAwaitMap[handlerKey]

					if available {
						if gossiper.debug {
							fmt.Println("__________Calling handler found for key ", handlerKey)
						}

						handler(*reply)
					}
				}
			} else {
				if gossiper.debug {
					fmt.Println("__________No handlers found for file name ", results[0].FileName)
				}
			}

		}
	} else {
		reply.HopLimit -= 1
		if reply.HopLimit > 0 {
			gossiper.forwardSearchReply(reply)
		}
	}
}

func searchRequestHandler(packet GossipPacket, gossiper *Gossiper) {
	request := packet.SearchRequest
	if gossiper.debug {
		fmt.Println("__________Received search request ", request.Origin, request.Keywords, request.Budget)
	}
	key := fmt.Sprintf("%s-%v", request.Origin, request.Keywords)
	recordedMillis, found := gossiper.lastSearchRequest[key]
	nowInMillis := time.Now().UnixNano() / 1000000
	diff := nowInMillis - recordedMillis
	gossiper.lastSearchRequest[key] = nowInMillis
	if found && diff < 500 {
		if gossiper.debug {
			fmt.Println("Duplicate search request received from", key)
		}
	} else {
		matches := []*SearchResult{}
		for _, keyword := range request.Keywords {
			pattern := fmt.Sprintf("^.*%s.*$", regexp.QuoteMeta(keyword))

			for _, file := range gossiper.fileList {
				isMatch, _ := regexp.MatchString(pattern, file.Name)
				if gossiper.debug {
					fmt.Printf("File name %s for pattern %s isMatch %t\n", file.Name, pattern, isMatch)
				}
				if isMatch {
					chunkMap := []uint64{}
					chunkCount := uint64(0)
					metaFile := file.MetaFile

					fromIndex := 0
					for fromIndex != len(metaFile) {
						endIndex := fromIndex + 32
						if endIndex > len(metaFile) {
							endIndex = len(metaFile)
						}

						hashChunk := metaFile[fromIndex:endIndex]
						hashChunkHex := hex.EncodeToString(hashChunk)

						_, found := gossiper.fileContentMap[hashChunkHex]

						if found {
							chunkMap = append(chunkMap, uint64(fromIndex/32)+1)
						}

						chunkCount += 1
						fromIndex = endIndex
					}

					result := SearchResult{
						FileName:     file.Name,
						MetafileHash: file.MetaHash,
						ChunkMap:     chunkMap,
						ChunkCount:   chunkCount,
					}
					matches = append(matches, &result)
				}
			}
		}

		if gossiper.debug {
			fmt.Println("__________Found matches ", len(matches), "for search ", request.Keywords)
		}
		if len(matches) > 0 {
			reply := SearchReply{Origin: gossiper.Name, Destination: request.Origin, HopLimit: 10, Results: matches}
			gossiper.forwardSearchReply(&reply)
		}

		request.Budget -= 1
		if request.Budget > 0 {
			gossiper.redistributeSearchRequest(request)
		}
	}
}

func dataReplyHandler(packet GossipPacket, gossiper *Gossiper) {
	reply := packet.DataReply
	if gossiper.debug {
		fmt.Printf("__________DataReply received dest %s origin %s hash %s hop-limit %d\n", reply.Destination, reply.Origin, reply.HashValue, reply.HopLimit)
	}
	if reply.Destination == gossiper.Name {
		isValid := validateDataReply(reply)
		if isValid {
			handler, available := gossiper.fileAwaitMap[hex.EncodeToString(reply.HashValue)]
			if available {
				if gossiper.debug {
					fmt.Println("__________Calling handle found for hash ", reply.HashValue)
				}

				handler(*reply)
			}
		} else {
			if gossiper.debug {
				fmt.Println("__________Chunk reply data and hash doesn't match. So dropping the packet.")
			}
		}
	} else {
		reply.HopLimit -= 1
		if reply.HopLimit > 0 {
			gossiper.forwardDataReply(reply)
		}
	}
}

func dataRequestHandler(packet GossipPacket, gossiper *Gossiper) {
	request := packet.DataRequest
	if gossiper.debug {
		fmt.Printf("__________DataRequest received dest %s origin %s hash %s hop-limit %d\n",
			request.Destination, request.Origin, request.HashValue, request.HopLimit)
	}
	if request.Destination == gossiper.Name {
		data, available := gossiper.fileContentMap[hex.EncodeToString(request.HashValue)]

		if available {
			if gossiper.debug {
				fmt.Printf("__________Replying origin %s dest %s hash %s\n", gossiper.Name, request.Origin, request.HashValue)
			}

			reply := DataReply{Origin: gossiper.Name, Destination: request.Origin, HopLimit: 10, HashValue: request.HashValue, Data: data}
			gossiper.forwardDataReply(&reply)
		}
	} else {
		request.HopLimit -= 1
		if request.HopLimit > 0 {
			gossiper.forwardDataRequest(request)
		}
	}
}

func privateHandler(packet GossipPacket, gossiper *Gossiper) {
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
}

func statusHandler(packet GossipPacket, relayPeer string, gossiper *Gossiper) {
	printStatusMessageLog(packet, relayPeer)
	printPeersLog(gossiper.Peers)
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
}

func getSearchHandlerKeys(searchAwaitMap map[string]func(reply SearchReply), fileName string) (matchingKeys []string) {
	matchingKeys = []string{}

	for key := range searchAwaitMap {
		keywords := strings.Split(key, ",")
		for _, keyword := range keywords {
			pattern := fmt.Sprintf("^.*%s.*$", regexp.QuoteMeta(keyword))
			isMatch, _ := regexp.MatchString(pattern, fileName)

			if isMatch {
				matchingKeys = append(matchingKeys, key)
			}
		}
	}
	return
}

func txPublishHandler(packet GossipPacket, gossiper *Gossiper) {
	receivedTx := packet.TxPublish

	gossiper.blockChainEventLoop <- func() {
		for _, tx := range gossiper.publishedTxs {
			if tx.File.Name == receivedTx.File.Name {
				return
			}
		}

		_, found := gossiper.fileMetaMap[receivedTx.File.Name]

		if found {
			return
		}

		gossiper.txChannel <- *receivedTx

		packet.TxPublish.HopLimit -= 1
		if packet.TxPublish.HopLimit > 0 {
			gossiper.broadcast(packet)
		}
	}
}

func blockPublishHandler(packet GossipPacket, gossiper *Gossiper) {
	receivedBlock := packet.BlockPublish.Block

	if gossiper.debug {
		fmt.Print("__________Received Block from network. Hop limit ", packet.BlockPublish.HopLimit, " ")
		for _, tx := range receivedBlock.Transactions {
			fmt.Print(tx.File.Name, " ")
		}
		fmt.Print(" | ")
		fmt.Printf("%x\n", receivedBlock.Hash())
	}

	if !isValidBlock(receivedBlock) {
		return
	}

	// discard if already seen
	receivedBlockHash := receivedBlock.Hash()
	for _, block := range gossiper.blockChain {
		existingBlockHash := block.Hash()
		if bytes.Equal(existingBlockHash[:], receivedBlockHash[:]) {
			if gossiper.debug {
				fmt.Println("__________Block already in longest chain. So discarding")
			}
			return
		}
	}

	for _, fork := range gossiper.forks {
		for _, block := range fork {
			existingBlockHash := block.Hash()
			if bytes.Equal(existingBlockHash[:], receivedBlockHash[:]) {
				if gossiper.debug {
					fmt.Println("__________Block already in one of forks. So discarding")
				}
				return
			}
		}
	}

	for _, block := range gossiper.strayBlocks {
		existingBlockHash := block.Hash()
		if bytes.Equal(existingBlockHash[:], receivedBlockHash[:]) {
			if gossiper.debug {
				fmt.Println("__________Block already in stray blocks list. So discarding")
			}
			return
		}
	}

	gossiper.blockChainEventLoop <- func() {

		gossiper.processBlock(receivedBlock)
		gossiper.processStrayBlocks()

		packet.BlockPublish.HopLimit -= 1
		if packet.BlockPublish.HopLimit > 0 {
			gossiper.broadcast(packet)
		}
	}
}
