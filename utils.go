package main

import (
	"fmt"
	"sort"
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

func printClientMessageLog(message string) {
	fmt.Printf("CLIENT MESSAGE %s\n", message)
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

func printCoinFlippedLog(address string) {
	fmt.Printf("FLIPPED COIN sending rumor to %s\n", address)
}

func printDSDVLog(origin string, nextHop string) {
	fmt.Printf("DSDV %s %s\n", origin, nextHop)
}

func getNextWantId(messages []GenericMessage) (nextId uint32) {
	onlyGossips := getOnlyGossips(messages)

	nextId = uint32(len(onlyGossips) + 1)
	// This processing is unnecessary since there are no gaps. But just in case...
	var idx int
	for idx = range onlyGossips {
		if onlyGossips[idx].ID != uint32(idx+1) {
			nextId = uint32(idx + 1)
			break
		}
	}
	return
}

func getOnlyGossips(messages []GenericMessage) (onlyGossips []GenericMessage) {
	onlyGossips = []GenericMessage{}
	for _, message := range messages {
		if message.ID != 0 {
			onlyGossips = append(onlyGossips, message)
		}
	}

	// Messages should already be in order. But just in case...
	sort.Slice(onlyGossips, func(i, j int) bool {
		return onlyGossips[i].ID < onlyGossips[j].ID
	})
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
