package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/gob"
	"errors"
	"github.com/dedis/protobuf"
	"os"
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
}

type PKIRecord struct {
	PubKey []byte // Public key encoded in x509.PKCS1 format
	Owner  string
}

type PKITransaction struct {
	Record    *PKIRecord
	Signature []byte
}

type TxPublish2 struct {
	File           File
	PKITransaction *PKITransaction
	HopLimit       uint32
}

func NewPKITranscation(privKey *rsa.PrivateKey, pubKey *rsa.PublicKey, owner string) (*PKITransaction, error) {
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

	return &PKITransaction{
		Record:    record,
		Signature: sig,
	}, nil
}

func (tx *PKITransaction) Verify() bool {
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
