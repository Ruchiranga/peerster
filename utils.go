package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
)

func printStatusMessageLog(packet GossipPacket, relayPeer string) {
	statusStr := fmt.Sprintf("STATUS from %s", relayPeer)
	for _, peerStatus := range packet.Status.Want {
		statusStr += fmt.Sprintf(" peer %s nextID %d", peerStatus.Identifier, peerStatus.NextId)
	}
	fmt.Printf("%s\n", statusStr)
}

func printInSyncLog(relayPeer string) {
	fmt.Printf("IN SYNC WITH %s\n", relayPeer)
}

func printMongeringWithLog(address string) {
	fmt.Printf("MONGERING with %s\n", address)
}

func printClientMessageLog(message string) {
	fmt.Printf("CLIENT MESSAGE %s\n", message)
}

func printPeersLog(peers []string) {
	fmt.Printf("PEERS %s\n", strings.Join(peers, ","))
}

func printSimpleMessageLog(message SimpleMessage) {
	fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", message.OriginalName, message.RelayPeerAddr, message.Contents)
}

func printRumorMessageLog(message RumorMessage, relayPeer string) {
	fmt.Printf("RUMOR origin %s from %s ID %d contents %s\n", message.Origin, relayPeer,
		message.ID, message.Text)
}

func printPrivateMessageLog(message PrivateMessage) {
	fmt.Printf("PRIVATE origin %s hop-limit %d contents %s\n", message.Origin, message.HopLimit, message.Text)
}

func printMetaFileDownloadLog(file string, peer string) {
	fmt.Printf("DOWNLOADING metafile of %s from %s\n", file, peer)
}

func printFileChunkDownloadLog(file string, index int, peer string) {
	fmt.Printf("DOWNLOADING %s chunk %d from %s\n", file, index, peer)
}

func printFileReconstructLog(file string) {
	fmt.Printf("RECONSTRUCTED file %s\n", file)
}

func printFileIndexingLog(file string) {
	fmt.Printf("INDEXING file %s\n", file)
}

func printSearchFinishedLog() {
	fmt.Println("SEARCH FINISHED")
}

func printFileIndexingCompletedLog(file string, hash string) {
	fmt.Printf("INDEXED file %s metahash %s\n", file, hash)
}

func printCoinFlippedLog(address string) {
	fmt.Printf("FLIPPED COIN sending rumor to %s\n", address)
}

func printDSDVLog(origin string, nextHop string) {
	fmt.Printf("DSDV %s %s\n", origin, nextHop)
}

func printSearchResultLog(fileName string, origin string, metaHash []byte, chunkMap []uint64) {
	sort.Slice(chunkMap, func(i, j int) bool {
		return chunkMap[i] < chunkMap[j]
	})
	list := fmt.Sprintf("%d", chunkMap[0])
	for idx, chunk := range chunkMap {
		if idx == 0 {
			continue
		}
		list = fmt.Sprintf("%s,%d", list, chunk)
	}
	fmt.Printf("FOUND match %s at %s metafile=%s chunks=%s\n", fileName, origin, hex.EncodeToString(metaHash), list)
}

func getNextWantId(messages []GenericMessage) (nextId uint32) {
	onlyGossips := getOnlyGossips(messages)
	nextId = uint32(len(onlyGossips) + 1)
	return
}

func getOnlyGossips(messages []GenericMessage) (onlyGossips []GenericMessage) {
	onlyGossips = []GenericMessage{}
	for _, message := range messages {
		if message.ID != 0 {
			onlyGossips = append(onlyGossips, message)
		}
	}
	return
}

func getGenericMessageFromRumor(rumor RumorMessage) (message GenericMessage) {
	message = GenericMessage{Origin: rumor.Origin, ID: rumor.ID, Text: rumor.Text}
	return
}

func getGenericMessageFromPrivate(pm PrivateMessage) (message GenericMessage) {
	message = GenericMessage{Origin: pm.Origin, ID: pm.ID, Text: pm.Text}
	return
}

func getRumorFromGenericMessage(message GenericMessage) (rumor RumorMessage) {
	rumor = RumorMessage{Origin: message.Origin, ID: message.ID, Text: message.Text}
	return
}

func writeToFile(content []byte, name string) (writtenCount int) {
	file, err := os.Create(fmt.Sprintf("./_Downloads/%s", name))
	if err != nil {
		fmt.Printf("__________Failed to create file %s\n", name)
		return
	}
	defer file.Close()

	count, err := file.Write(content)
	if err != nil {
		fmt.Printf("__________Failed to write contents to file %s\n", name)
		return
	}
	writtenCount = count
	return
}

func validateDataReply(reply *DataReply) (valid bool) {
	replyDataHash := sha256.Sum256(reply.Data)
	if bytes.Equal(reply.HashValue, replyDataHash[:]) {
		return true
	}
	return false
}
