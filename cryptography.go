package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/dedis/protobuf"
	"os"
	"time"
)

const KeySize = 2048
const DefaultKeyFileName = "private.key"

/*
type EncDataRequest struct {
	Origin       string
	Destination  string
	HopLimit     uint32
	EncHashValue []byte
}

type EncDataReply struct {
	Origin       string
	Destination  string
	HopLimit     uint32
	EncHashValue []byte
	EncData      []byte
}
*/

type EncPrivateMessage struct {
	Origin      string
	ID          uint32
	EncText     []byte
	Destination string
	HopLimit    uint32
	Signature   []byte
	Temp        string
}

type PKIRecord struct {
	PubKey []byte // Public key encoded in x509.PKCS1 format
	Owner  string
}

type PKIAnnoucement struct {
	Record    *PKIRecord
	Signature []byte
}

type TxPublish2 struct {
	File           File
	PKITransaction *PKIAnnoucement
	HopLimit       uint32
}

func NewPKIAnnouncement(privKey *rsa.PrivateKey, pubKey *rsa.PublicKey, owner string) (*PKIAnnoucement, error) {
	encoded := EncodePublicKey(pubKey)

	record := &PKIRecord{
		PubKey: encoded,
		Owner:  owner,
	}

	buf, err := protobuf.Encode(record)
	if err != nil {
		return nil, err
	}

	sig, err := Sign(privKey, buf)
	if err != nil {
		return nil, err
	}

	return &PKIAnnoucement{
		Record:    record,
		Signature: sig,
	}, nil
}

func (tx *PKIAnnoucement) Verify() bool {
	msg, err := protobuf.Encode(tx.Record)
	if err != nil {
		return false
	}

	decoded, err := DecodePublicKey(tx.Record.PubKey)
	if err != nil {
		return false
	}

	return Verify(decoded, msg, tx.Signature)
}

func newKeyPair() (*rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, KeySize)
	return key, err
}

func GenKey(savepath string) (*rsa.PrivateKey, error) {
	key, err := newKeyPair()
	if err != nil {
		return nil, err
	}

	outFile, err := os.Create(savepath)
	if err != nil {
		return nil, err
	}
	defer outFile.Close()

	encoder := gob.NewEncoder(outFile)
	err = encoder.Encode(key)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func LoadKey(path string) (*rsa.PrivateKey, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	key := &rsa.PrivateKey{}
	decoder := gob.NewDecoder(reader)
	err = decoder.Decode(key)
	if err != nil {
		return nil, err
	}

	err = key.Validate()
	if err != nil {
		return nil, err
	}

	return key, nil
}

func GetKey(path string) (*rsa.PrivateKey, error) {
	key, err := LoadKey(path)
	if err != nil {
		return GenKey(path)
	}

	return key, err
}

func Encrypt(pubKey *rsa.PublicKey, msg []byte) ([]byte, error) {
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, pubKey, msg, []byte{})
}

func Decrypt(privKey *rsa.PrivateKey, ciphertext []byte) ([]byte, error) {
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, privKey, ciphertext, []byte{})
}

func Sign(privKey *rsa.PrivateKey, msg []byte) ([]byte, error) {
	h := sha256.New()
	h.Write(msg)

	return rsa.SignPSS(rand.Reader, privKey, crypto.SHA256, h.Sum(nil), nil)
}

func Verify(pubKey *rsa.PublicKey, msg []byte, sig []byte) bool {
	h := sha256.New()
	h.Write(msg)

	err := rsa.VerifyPSS(pubKey, crypto.SHA256, h.Sum(nil), sig, nil)

	return err == nil
}

func EncodePublicKey(pubKey *rsa.PublicKey) []byte {
	return x509.MarshalPKCS1PublicKey(pubKey)
}

func DecodePublicKey(encoded []byte) (*rsa.PublicKey, error) {
	key, err := x509.ParsePKCS1PublicKey(encoded)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func NewEncPrivateMsg(origin string, text string, dest string, originPrivK *rsa.PrivateKey, destPubK *rsa.PublicKey) (*EncPrivateMessage, error) {
	enc, err := Encrypt(destPubK, []byte(text))
	if err != nil {
		return nil, err
	}

	content := append(enc, []byte(origin+dest)...)
	sig, err := Sign(originPrivK, content)
	if err != nil {
		return nil, err
	}

	epm := &EncPrivateMessage{
		Origin:      origin,
		ID:          0,
		EncText:     enc,
		Destination: dest,
		HopLimit:    10,
		Signature:   sig,
	}

	return epm, nil
}

func (epm *EncPrivateMessage) getContent() []byte {
	return append(epm.EncText, []byte(epm.Origin+epm.Destination)...)
}

func (epm *EncPrivateMessage) VerifyAndDecrypt(originPubK *rsa.PublicKey, destPrivK *rsa.PrivateKey) (string, error) {
	content := epm.getContent()
	if !Verify(originPubK, content, epm.Signature) {
		return "", errors.New("Invalid signature")
	}

	dec, err := Decrypt(destPrivK, epm.EncText)
	if err != nil {
		return "", err
	}

	return string(dec), nil
}

func (gossiper *Gossiper) initKey(path string) {
	if key, err := GetKey(path); err == nil {
		gossiper.key = key
	}
}

func (gossiper *Gossiper) txPublishMyKey(privKey *rsa.PrivateKey, name string) {
	if ann, err := NewPKIAnnouncement(privKey, &privKey.PublicKey, name); err == nil {
		txPub := TxPublish{Announcement: ann, HopLimit: 1}
		gossiper.txChannel <- txPub
		gossiper.broadcast(GossipPacket{TxPublish: &txPub})
	}
}

func (announcement *PKIAnnoucement) Equal(other *PKIAnnoucement) bool {
	return announcement.Record.Owner == other.Record.Owner && bytes.Equal(announcement.Record.PubKey, other.Record.PubKey)
}

func encPrivateHandler(packet GossipPacket, gossiper *Gossiper) {
	epm := packet.EncPrivate
	if epm.Destination == gossiper.Name {

		originPubK, ok := gossiper.keyMap[epm.Origin]
		if !ok {
			return
		}

		text, err := epm.VerifyAndDecrypt(originPubK, gossiper.key)
		if err != nil {
			fmt.Println(err)
			return
		}

		printEncPrivate(epm.Origin, epm.HopLimit, text)

		genericMessage := GenericMessage{Origin: epm.Origin, ID: epm.ID, Text: text}
		gossiper.storeMessage(genericMessage)

	} else {
		epm.HopLimit -= 1
		if epm.HopLimit > 0 {
			gossiper.forwardEncPrivateMessage(epm)
		}
	}
}

func (gossiper *Gossiper) forwardEncPrivateMessage(encPrivate *EncPrivateMessage) {
	address, found := gossiper.routingTable[encPrivate.Destination]
	if found {
		packet := GossipPacket{EncPrivate: encPrivate}
		gossiper.writeToAddr(packet, address)
	} else {
		if gossiper.debug {
			fmt.Printf("__________Failed to forward private message. Next hop for %s not found.\n", encPrivate.Destination)
		}
	}
}

func printEncPrivate(origin string, hl uint32, contents string) {
	fmt.Println("ENC PRIVATE origin " + origin + " hop-limit " + fmt.Sprint(hl) + " contents " + contents)
}

type BlockChainRequest struct {
	Origin string
	//HopLimit    uint32
}

type BlockChainReply struct {
	Origin      string
	Destination string
	//HopLimit    uint32
	Blockchain []Block
}

func blockchainRequestHandler(packet GossipPacket, gossiper *Gossiper) {
	bcReq := packet.BlockChainRequest

	fmt.Println("RECEIVED BLOCKCHAIN REQUEST")

	bcRep := &BlockChainReply{
		Origin:      gossiper.Name,
		Destination: bcReq.Origin,
		Blockchain:  gossiper.blockChain,
	}

	gossiper.forwardBlockchainReply(bcRep)
}

func (gossiper *Gossiper) forwardBlockchainReply(bcRep *BlockChainReply) {
	address, found := gossiper.routingTable[bcRep.Destination]
	if found {
		packet := GossipPacket{BlockChainReply: bcRep}
		gossiper.writeToAddr(packet, address)
	} else {
		fmt.Printf("__________Failed to forward blockchain reply. Next hop for %s not found.\n", bcRep.Destination)
		if gossiper.debug {
			fmt.Printf("__________Failed to forward blockchain reply. Next hop for %s not found.\n", bcRep.Destination)
		}
	}
}

func blockchainReplyHandler(packet GossipPacket, gossiper *Gossiper) {
	bcRep := packet.BlockChainReply

	fmt.Println("RECEIVED BLOCKCHAIN REPLY")

	if bcRep.Destination == gossiper.Name && !gossiper.blockchainBootstrap {
		gossiper.blockchainBootstrap = true

		gossiper.fileMetaMap = make(map[string][]byte)
		gossiper.keyMap = make(map[string]*rsa.PublicKey)

		gossiper.blockChain = bcRep.Blockchain

		for _, block := range bcRep.Blockchain {
			for _, tx := range block.Transactions {
				if ann := tx.Announcement; ann != nil {
					if key, err := DecodePublicKey(ann.Record.PubKey); err == nil && ann.Verify() {
						gossiper.keyMap[ann.Record.Owner] = key
						fmt.Println("FOUND KEY FROM BOOTSTRAP " + ann.Record.Owner + " " + hex.EncodeToString(ann.Record.PubKey))
					}
				} else {
					gossiper.fileMetaMap[tx.File.Name] = tx.File.MetafileHash
				}
			}
		}
	}
}

func (gossiper *Gossiper) bootstrapBlockchain() {
	<-time.After(5 * time.Second)

	packet := GossipPacket{
		BlockChainRequest: &BlockChainRequest{
			Origin: gossiper.Name,
		},
	}

	for _, peer := range gossiper.Peers {
		gossiper.writeToAddr(packet, peer)
	}
}
