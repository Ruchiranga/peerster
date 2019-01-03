package main

import (
	"crypto/sha256"
	"encoding/binary"
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

type PrivateMessage struct {
	Origin      string
	ID          uint32
	Text        string
	Destination string
	HopLimit    uint32
}

type GossipPacket struct {
	Simple                *SimpleMessage
	Rumor                 *RumorMessage
	Status                *StatusPacket
	Private               *PrivateMessage
	DataRequest           *DataRequest
	DataReply             *DataReply
	SearchRequest         *SearchRequest
	SearchReply           *SearchReply
	TxPublish             *TxPublish
	BlockPublish          *BlockPublish
	NeighbourNotification *NeighbourNotification
	NotificationResponse  *NotificationResponse
	FilePullRequest       *FilePullRequest
	FileReplicateAck      *FileReplicateAck
	EncPrivate            *EncPrivateMessage
	BlockChainRequest     *BlockChainRequest
	BlockChainReply       *BlockChainReply
}

type GenericMessage struct {
	Origin string
	ID     uint32
	Text   string
}

type DataRequest struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
}

type DataReply struct {
	Origin        string
	Destination   string
	HopLimit      uint32
	HashValue     []byte
	Data          []byte
	StreamableSrc string
}

type SearchRequest struct {
	Origin   string
	Budget   uint64
	Keywords []string
}

type SearchReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	Results     []*SearchResult
}

type SearchResult struct {
	FileName      string
	MetafileHash  []byte
	ChunkMap      []uint64
	ChunkCount    uint64
	StreamableSrc string
}

type File struct {
	Name          string
	Size          int64
	MetafileHash  []byte
	StreamableSrc string
}

type TxPublish struct {
	File         File
	Announcement *PKIAnnoucement
	HopLimit     uint32
}

type BlockPublish struct {
	Block    Block
	HopLimit uint32
}

type Block struct {
	PrevHash     [32]byte
	Nonce        [32]byte
	Transactions []TxPublish
}

type NeighbourNotification struct {
	Origin      string
	Address     string
	Destination string
}

type FilePullRequest struct {
	Origin      string
	Destination string
	HashValue   []byte
	FileName    string
	FileId      string
}

type FileReplicateAck struct {
	Destination string
	HashValue   []byte
	Origin      string
}

type NotificationResponse struct {
	Destination  string
	Origin       string
	Address      string
	Neighbors    []Peer
	Level        int
	Row          []Peer
	UpperLeafSet []Peer
	LowerLeafSet []Peer
	killThyself  bool
}

type Peer struct {
	Name    string
	Address string
}

func (peer *Peer) isInitialized() (result bool) {
	if len(peer.Name) == NodeIDLength && len(peer.Address) > 0 {
		return true
	}
	return false
}

func (b *Block) Hash() (out [32]byte) {
	h := sha256.New()
	h.Write(b.PrevHash[:])
	h.Write(b.Nonce[:])
	binary.Write(h, binary.LittleEndian,
		uint32(len(b.Transactions)))
	for _, t := range b.Transactions {
		th := t.Hash()
		h.Write(th[:])
	}
	copy(out[:], h.Sum(nil))
	return
}

func (t *TxPublish) Hash() (out [32]byte) {
	h := sha256.New()
	binary.Write(h, binary.LittleEndian,
		uint32(len(t.File.Name)))
	h.Write([]byte(t.File.Name))
	h.Write(t.File.MetafileHash)

	if (t.Announcement != nil) {
		binary.Write(h, binary.LittleEndian, uint32(len(t.Announcement.Record.Owner)))
		h.Write([]byte(t.Announcement.Record.Owner))
		h.Write(t.Announcement.Record.PubKey)
		h.Write(t.Announcement.Signature)
	}

	copy(out[:], h.Sum(nil))
	return
}
