package main

import "fmt"

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