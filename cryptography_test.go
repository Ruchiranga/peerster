package main

import (
	"bytes"
	"fmt"
	"github.com/dedis/protobuf"
	"testing"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func TestCryptography_GenKey(t *testing.T) {
	path := "private.key"

	key, err := GenKey(path)

	assertEqual(t, key != nil, true)
	assertEqual(t, err == nil, true)
}

func TestCryptography_LoadKey(t *testing.T) {
	path := "private.key"

	_, err := GenKey(path)

	loadedKey, err := LoadKey(path)
	assertEqual(t, loadedKey != nil, true)
	assertEqual(t, err == nil, true)
}

func TestCryptography_GetKey(t *testing.T) {
	path := "private.key"

	// GetKey generates the key
	loadedKey, err := GetKey(path)
	assertEqual(t, loadedKey != nil, true)
	assertEqual(t, err == nil, true)

	// GetKey loads the key
	loadedKey2, err := GetKey(path)
	assertEqual(t, loadedKey2 != nil, true)
	assertEqual(t, err == nil, true)

	msg := []byte("test")
	enc, _ := Encrypt(&loadedKey.PublicKey, msg)
	dec, _ := Decrypt(loadedKey2, enc)
	assertEqual(t, bytes.Equal(msg, dec), true)

	enc2, _ := Encrypt(&loadedKey2.PublicKey, msg)
	dec2, _ := Decrypt(loadedKey, enc2)
	assertEqual(t, bytes.Equal(msg, dec2), true)
}

func TestCryptography_EncryptAndDecrypt(t *testing.T) {
	path := "private.key"

	privKey, _ := GenKey(path)
	pubKey := &privKey.PublicKey

	msg := []byte("test")

	ciphertext, err := Encrypt(pubKey, msg)
	assertEqual(t, err == nil, true)

	decrypted, err := Decrypt(privKey, ciphertext)
	assertEqual(t, err == nil, true)
	assertEqual(t, bytes.Equal(msg, decrypted), true)
}

func TestCryptography_SignAndVerify(t *testing.T) {
	path := "private.key"

	privKey, _ := GenKey(path)
	pubKey := &privKey.PublicKey

	msg := []byte("test")

	msgTampered := []byte("test2")

	fmt.Println(msg, msgTampered)

	sig, err := Sign(privKey, msg)
	assertEqual(t, sig != nil, true)
	assertEqual(t, err == nil, true)

	valid := Verify(pubKey, msg, sig)
	assertEqual(t, valid, true)

	valid = Verify(pubKey, msgTampered, sig)
	assertEqual(t, valid, false)
}

func TestCryptography_EncodeAndDecodePubKey(t *testing.T) {
	path := "private.key"

	privKey, _ := GenKey(path)
	pubKey := &privKey.PublicKey

	encoded := EncodePublicKey(pubKey)
	_, err := DecodePublicKey(encoded)

	assertEqual(t, err, nil)
}

func TestCrytography_PKITransactionMarshalling(t *testing.T) {
	path := "private.key"

	privKey, _ := GenKey(path)
	pubKey := &privKey.PublicKey
	encoded := EncodePublicKey(pubKey)

	owner := "nodeA"

	record := &PKIRecord{
		PubKey: encoded,
		Owner:  owner,
	}

	buf, err := protobuf.Encode(record)
	fmt.Println(err)
	assertEqual(t, err == nil, true)

	sig, err := Sign(privKey, buf)

	tx := &PKIAnnoucement{
		Record:    record,
		Signature: sig,
	}
	buf, err = protobuf.Encode(tx)
	assertEqual(t, err == nil, true)

	decoded := &PKIAnnoucement{}
	err = protobuf.Decode(buf, decoded)
	assertEqual(t, err == nil, true)
	assertEqual(t, bytes.Equal(decoded.Signature, sig), true)
	assertEqual(t, decoded.Record.Owner, owner)
	assertEqual(t, bytes.Equal(decoded.Record.PubKey, encoded), true)
}

func TestCryptography_PKITranscationVerify(t *testing.T) {
	path := "private.key"

	privKey, _ := GenKey(path)
	pubKey := &privKey.PublicKey

	owner := "Alice"

	tx, err := NewPKIAnnouncement(privKey, pubKey, owner)
	assertEqual(t, err == nil, true)

	assertEqual(t, tx.Verify(), true)

	// Eve tampers with the mapping, claiming it's her public key
	tx.Record.Owner = "Eve"
	assertEqual(t, tx.Verify(), false)

	// Eve tries again, but this time, she changes the signature with one created from her private key
	pathEve := "eve.key"
	privKeyEve, _ := GenKey(pathEve)

	badTX, _ := NewPKIAnnouncement(privKeyEve, pubKey, "Eve")
	assertEqual(t, badTX.Verify(), false)
}
