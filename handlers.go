package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func fileReplicateAckHandler(packet GossipPacket, gossiper *Gossiper) {
	ack := packet.FileReplicateAck
	if gossiper.debug {
		fmt.Println("__________Received file replicate ack", ack)
	}
	if ack.Destination == gossiper.Name {
		handler, available := gossiper.fileReplicateAwaitMap[hex.EncodeToString(ack.HashValue)]
		if available {
			if gossiper.debug {
				fmt.Println("__________Calling handle found for hash ", ack.HashValue)
			}

			handler(*ack)
		} else {
			if gossiper.debug {
				fmt.Println("__________Got file replicate ack but no handler found")
			}
		}
	} else {
		if gossiper.debug {
			fmt.Println("__________Ack not for me. passing on.")
		}
		gossiper.forwardFileReplicateAck(ack, 0)
	}
}

func filePullRequestHandler(packet GossipPacket, gossiper *Gossiper) {
	pullRequest := packet.FilePullRequest
	if gossiper.debug {
		fmt.Println("__________File Pull Request received", pullRequest)
	}

	if pullRequest.Destination == gossiper.Name {
		if gossiper.debug {
			fmt.Println("__________PR is for me. Checking next dest")
		}
		nextDestination := getNumericallyClosestId(pullRequest.FileId, gossiper)
		if nextDestination == gossiper.Name {
			if gossiper.debug {
				fmt.Println("__________Pull request destined for me. Initiating file download")
			}
			statusChan := make(chan bool)
			metaHashHex := hex.EncodeToString(pullRequest.HashValue)
			fileName := fmt.Sprintf("%s-replicate-%s", pullRequest.FileName, gossiper.Name)
			gossiper.requestFile(metaHashHex, pullRequest.Origin, fileName, statusChan)
			go func() {
				status := <-statusChan
				if status {
					printReplicateDownloadSuccessLog(pullRequest.FileName, metaHashHex)
					ack := FileReplicateAck{Destination: pullRequest.Origin, HashValue: pullRequest.HashValue, Origin: gossiper.Name}
					if gossiper.debug {
						fmt.Println("__________Sending back file replicate ack for", pullRequest.FileName)
					}
					gossiper.forwardFileReplicateAck(&ack, 0)
				} else {
					if gossiper.debug {
						fmt.Println("__________File downloaded failed")
					}
				}
			}()
		} else {
			if gossiper.debug {
				fmt.Println("__________Forwarded PR to", nextDestination)
			}
			pullRequest.Destination = nextDestination
			gossiper.forwardFilePullRequest(pullRequest, 0)
		}
	} else {
		if gossiper.debug {
			fmt.Println("__________PR not for me. Passing on")
		}
		gossiper.forwardFilePullRequest(pullRequest, 0)
	}
}

func neighbourNotificationHandler(packet GossipPacket, gossiper *Gossiper) {
	notification := packet.NeighbourNotification
	if gossiper.debug {
		fmt.Println("__________Notification received", notification)
	}
	if notification.Destination == gossiper.Name || notification.Destination == "" {
		sharedLength := getSharedLength(gossiper.Name, notification.Origin)
		if gossiper.debug {
			fmt.Printf("__________Shared length %d\n", sharedLength)
		}
		var response NotificationResponse

		if sharedLength == NodeIDLength {
			response = NotificationResponse{
				Destination: notification.Origin,
				killThyself: true,
			}
			fmt.Println("Forwarding kill thyself response")
			gossiper.forwardNotificationResponse(&response, 0)
			return
		} else {
			response = NotificationResponse{
				Origin:      gossiper.Name,
				Address:     gossiper.gossipAddress.String(),
				Level:       sharedLength,
				Row:         gossiper.pastryRoutingTable[sharedLength][:],
				Destination: notification.Origin,
			}

			nextDestination := getNumericallyClosestId(notification.Origin, gossiper)
			if nextDestination == gossiper.Name {
				if gossiper.debug {
					fmt.Println("__________Notification is destined for me!. Including my leaf set in response")
				}
				response.UpperLeafSet = gossiper.upperLeafSet
				response.LowerLeafSet = gossiper.lowerLeafSet
			} else {
				if nextDestination != "" {
					notification.Destination = nextDestination
					if gossiper.debug {
						fmt.Printf("__________Forwarding notification to next dest %s\n", nextDestination)
					}
					gossiper.forwardNeighbourNotification(notification, 0)
				} else {
					if gossiper.debug {
						fmt.Println("__________Next destination not found!")
					}
				}

			}

			if sharedLength == 0 {
				response.Neighbors = gossiper.neighbours
			}

			if gossiper.debug {
				fmt.Println("__________Forwarding response", response)
			}
			gossiper.forwardNotificationResponse(&response, 0)
		}

		if notification.Origin != gossiper.Name {
			storePeerInState(Peer{Name: notification.Origin, Address: notification.Address}, gossiper)
			gossiper.printPastryState()
		}
	} else {
		if gossiper.debug {
			fmt.Println("__________Not for me. Passing on.")
		}
		gossiper.forwardNeighbourNotification(notification, 0)
	}

}

func storePeerInState(peer Peer, gossiper *Gossiper) {
	if peer.Name == gossiper.Name {
		fmt.Println("__________In storePeerInState, passed peer is me. returning.")
		return
	}

	// Check if this is a neighbour
	for _, peerAddress := range gossiper.Peers {
		if peerAddress == peer.Address && len(gossiper.neighbours) < cap(gossiper.neighbours) {
			peerExists := isPeerExistingIn(peer, gossiper.neighbours)
			if !peerExists {
				gossiper.neighbours = append(gossiper.neighbours, peer)
			} else {
				if gossiper.debug {
					fmt.Println("__________Peer already exists in neighbours", peer)
				}
			}
		}
	}

	// Check if it can go into routing table
	sharedLength := getSharedLength(gossiper.Name, peer.Name)
	nameChar := string(peer.Name[sharedLength])
	nameCharVal, _ := strconv.ParseInt(nameChar, 4, 0)
	routingNode := gossiper.pastryRoutingTable[sharedLength][nameCharVal]
	if !routingNode.isInitialized() {
		gossiper.pastryRoutingTable[sharedLength][nameCharVal] = peer
	}

	// Check if it can go to leaf set
	peerNameVal, _ := strconv.ParseInt(peer.Name, 4, 0)
	thisNodeNameVal, _ := strconv.ParseInt(gossiper.Name, 4, 0)

	if peerNameVal > thisNodeNameVal {
		peerExists := isPeerExistingIn(peer, gossiper.upperLeafSet)
		if !peerExists {
			if len(gossiper.upperLeafSet) < cap(gossiper.upperLeafSet) {
				gossiper.upperLeafSet = append(gossiper.upperLeafSet, peer)
			} else {
				sort.Slice(gossiper.upperLeafSet, func(i, j int) bool {
					iVal, _ := strconv.ParseInt(gossiper.upperLeafSet[i].Name, 4, 0)
					jVal, _ := strconv.ParseInt(gossiper.upperLeafSet[j].Name, 4, 0)
					return iVal < jVal
				})
				largestLeaf := gossiper.upperLeafSet[len(gossiper.upperLeafSet)-1]
				largestLeafVal, _ := strconv.ParseInt(largestLeaf.Name, 4, 0)

				if peerNameVal < largestLeafVal {
					gossiper.upperLeafSet[len(gossiper.upperLeafSet)-1] = peer
				}
			}
		} else {
			if gossiper.debug {
				fmt.Println("__________Peer already exists in upper leaf set")
			}
		}
	} else if peerNameVal < thisNodeNameVal {
		peerExists := isPeerExistingIn(peer, gossiper.lowerLeafSet)
		if !peerExists {
			if len(gossiper.lowerLeafSet) < cap(gossiper.lowerLeafSet) {
				gossiper.lowerLeafSet = append(gossiper.lowerLeafSet, peer)
			} else {
				sort.Slice(gossiper.lowerLeafSet, func(i, j int) bool {
					iVal, _ := strconv.ParseInt(gossiper.lowerLeafSet[i].Name, 4, 0)
					jVal, _ := strconv.ParseInt(gossiper.lowerLeafSet[j].Name, 4, 0)
					return iVal < jVal
				})
				smallestLeaf := gossiper.lowerLeafSet[0]
				smallestLeafVal, _ := strconv.ParseInt(smallestLeaf.Name, 4, 0)

				if peerNameVal > smallestLeafVal {
					gossiper.lowerLeafSet[0] = peer
				}
			}
		} else {
			if gossiper.debug {
				fmt.Println("__________Peer already exists in lower leaf set")
			}
		}
	} else {
		fmt.Println("__________Same node name value. Cannot happen!")
	}
}

func getNumericallyClosestId(name string, gossiper *Gossiper) (closestId string) {
	closestId = gossiper.Name

	nameVal, _ := strconv.ParseInt(name, 4, 0)
	thisNodeNameVal, _ := strconv.ParseInt(gossiper.Name, 4, 0)

	leafSet := append(gossiper.lowerLeafSet, gossiper.upperLeafSet...)

	if len(leafSet) > 1 {
		sort.Slice(leafSet, func(i, j int) bool {
			iVal, _ := strconv.ParseInt(leafSet[i].Name, 4, 0)
			jVal, _ := strconv.ParseInt(leafSet[j].Name, 4, 0)
			return iVal < jVal
		})
		minLeaf := leafSet[0]
		maxLeaf := leafSet[len(leafSet)-1]

		minLeafVal, _ := strconv.ParseInt(minLeaf.Name, 4, 0)
		maxLeafVal, _ := strconv.ParseInt(maxLeaf.Name, 4, 0)

		leastDif := int64(65535)

		if nameVal > minLeafVal && nameVal < maxLeafVal {
			for _, leaf := range leafSet {
				leafVal, _ := strconv.ParseInt(leaf.Name, 4, 0)
				dif := Abs(leafVal - nameVal)
				if dif < leastDif {
					leastDif = dif
					closestId = leaf.Name
				}
			}

			dif := Abs(thisNodeNameVal - nameVal)
			// If this node is the closest
			if dif < leastDif {
				closestId = gossiper.Name
			} else {
				if gossiper.debug {
					fmt.Println("__________Picked closest id from leafs")
				}
			}
			return
		}
	}

	sharedLength := getSharedLength(gossiper.Name, name)
	nameChar := string(name[sharedLength])
	nameCharVal, _ := strconv.ParseInt(nameChar, 4, 0)
	routingNode := gossiper.pastryRoutingTable[sharedLength][nameCharVal]
	if routingNode.isInitialized() {
		closestId = routingNode.Name
		if gossiper.debug {
			fmt.Println("__________Picked closest id from routing table")
		}
		return
	} else {
		for _, leaf := range gossiper.lowerLeafSet {
			leafVal, _ := strconv.ParseInt(leaf.Name, 4, 0)
			if getSharedLength(leaf.Name, name) >= sharedLength && Abs(thisNodeNameVal-nameVal) > Abs(leafVal-nameVal) {
				closestId = leaf.Name
				if gossiper.debug {
					fmt.Println("__________Closest ID from Last resort. From Lower Leaf Set")
				}
				return
			}
		}

		for _, leaf := range gossiper.upperLeafSet {
			leafVal, _ := strconv.ParseInt(leaf.Name, 4, 0)
			if getSharedLength(leaf.Name, name) >= sharedLength && Abs(thisNodeNameVal-nameVal) > Abs(leafVal-nameVal) {
				closestId = leaf.Name
				if gossiper.debug {
					fmt.Println("__________Closest ID from Last resort. From Upper Leaf Set")
				}
				return
			}
		}

		for oi := range gossiper.pastryRoutingTable {
			for ii := range gossiper.pastryRoutingTable[oi] {
				node := gossiper.pastryRoutingTable[oi][ii]
				if node.isInitialized() {
					nodeVal, _ := strconv.ParseInt(node.Name, 4, 0)
					if getSharedLength(node.Name, name) >= sharedLength && Abs(thisNodeNameVal-nameVal) > Abs(nodeVal-nameVal) {
						closestId = node.Name
						if gossiper.debug {
							fmt.Println("__________Closest ID from Last resort. From routing table")
						}
						return
					}
				}
			}
		}

		for _, node := range gossiper.neighbours {
			nodeVal, _ := strconv.ParseInt(node.Name, 4, 0)
			if getSharedLength(node.Name, name) >= sharedLength && Abs(thisNodeNameVal-nameVal) > Abs(nodeVal-nameVal) {
				closestId = node.Name
				if gossiper.debug {
					fmt.Println("__________Closest ID from Last resort. From neighbours")
				}

				return
			}
		}
	}
	if gossiper.debug {
		fmt.Println("__________Closest ID not found. Returning myself")
	}

	return
}

func notificationResponseHandler(packet GossipPacket, gossiper *Gossiper) {
	response := packet.NotificationResponse
	if gossiper.debug {
		fmt.Println("__________Notification Response received ", *response)
	}
	if response.killThyself {
		// Chosen ID non unique. Killing process.
		fmt.Println("__________Chosen Node ID (name) found to be non unique. Killing the process. Pls restart")
		os.Exit(3)
	}

	if response.Destination == gossiper.Name {
		if response.Row != nil {
			for _, entry := range response.Row {
				if entry.isInitialized() {
					storePeerInState(entry, gossiper)
				}
			}
		} else {
			fmt.Println("__________Response Row is nil. Should not happen!")
		}

		if response.Neighbors != nil {
			for _, theirNeighbour := range response.Neighbors {
				storePeerInState(theirNeighbour, gossiper)
			}
		}
		if response.UpperLeafSet != nil {
			for _, theirLeaf := range response.UpperLeafSet {
				storePeerInState(theirLeaf, gossiper)
			}
		}
		if response.LowerLeafSet != nil {
			for _, theirLeaf := range response.LowerLeafSet {
				storePeerInState(theirLeaf, gossiper)
			}
		}

		if response.Origin != gossiper.Name {
			storePeerInState(Peer{Name: response.Origin, Address: response.Address}, gossiper)
			gossiper.printPastryState()
		}
	} else {
		if gossiper.debug {
			fmt.Println("__________Not for me. Passing on.")
		}
		gossiper.forwardNotificationResponse(response, 0)
	}

}

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
						FileName:      file.Name,
						MetafileHash:  file.MetaHash,
						ChunkMap:      chunkMap,
						ChunkCount:    chunkCount,
						StreamableSrc: file.StreamableSrc,
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
		streamableSrc, _ := gossiper.fileStreamableSrcMap[hex.EncodeToString(request.HashValue)]

		if available {
			if gossiper.debug {
				fmt.Printf("__________Replying origin %s dest %s hash %s\n", gossiper.Name, request.Origin, request.HashValue)
			}

			reply := DataReply{Origin: gossiper.Name, Destination: request.Origin, HopLimit: 10, HashValue: request.HashValue, Data: data, StreamableSrc: streamableSrc}
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

	fmt.Println("Received txpub from:", receivedTx.Announcement.Record.Owner)

	gossiper.blockChainEventLoop <- func() {
		if receivedTx.Announcement != nil {
			if !receivedTx.Announcement.Verify() {
				fmt.Println("INVALID ANNOUNCEMENT")
				return
			}
		} else {
			for _, tx := range gossiper.publishedTxs {
				if tx.File.Name == receivedTx.File.Name {
					return
				}
			}

			_, found := gossiper.fileMetaMap[receivedTx.File.Name]

			if found {
				return
			}
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
