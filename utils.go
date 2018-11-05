package main

import (
	"fmt"
	"log"
	"os"
)

func printStatusMessageLog(packet GossipPacket, relayPeer string) {
	//statusStr := fmt.Sprintf("STATUS from %s", relayPeer)
	//for _, peerStatus := range packet.Status.Want {
	//	statusStr += fmt.Sprintf(" peer %s nextID %d", peerStatus.Identifier, peerStatus.NextId)
	//}
	//fmt.Printf("%s\n", statusStr)
}

func printInSyncLog(relayPeer string) {
	//fmt.Printf("IN SYNC WITH %s\n", relayPeer)
}

func printMongeringWithLog(address string) {
	//fmt.Printf("MONGERING with %s\n", address)
}

func printClientMessageLog(message string) {
	fmt.Printf("CLIENT MESSAGE %s\n", message)
}

func printSimpleMessageLog(message SimpleMessage) {
	fmt.Printf("SIMPLE MESSAGE origin %s from %s contents %s\n", message.OriginalName, message.RelayPeerAddr, message.Contents)
}

func printRumorMessageLog(message RumorMessage, relayPeer string) {
	//fmt.Printf("RUMOR origin %s from %s ID %d contents %s\n", message.Origin, relayPeer,
	//	message.ID, message.Text)
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

func printCoinFlippedLog(address string) {
	//fmt.Printf("FLIPPED COIN sending rumor to %s\n", address)
}

func printDSDVLog(origin string, nextHop string) {
	//fmt.Printf("DSDV %s %s\n", origin, nextHop)
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

func writeToFile(content []byte, name string) {
	file, err := os.Create(fmt.Sprintf("./_Downloads/%s", name))
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to create file %s", name))
		return
	}
	defer file.Close()

	count, err := file.Write(content)
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to write contents to file %s", name))
		return
	}
	fmt.Printf("Wrote %d bytes to file. Content size is %d", count, len(content))
}
