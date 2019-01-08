package main

import (
	"fmt"
	"os"
)

const BCLogDefaultPath = "bc.log"
const MaliciousLogDefaultPath = "mal.log"

func createBlockchainLog(prefix string) (string, error) {
	path := prefix + "-" + BCLogDefaultPath
	f, err := os.Create(path)
	defer f.Close()

	if err != nil {
		return "", err
	}

	return path, nil
}

func createMaliciousLog(prefix string) (string, error) {
	path := prefix + "-" + MaliciousLogDefaultPath
	f, err := os.Create(path)
	defer f.Close()

	if err != nil {
		return "", err
	}

	return path, nil
}

func OpenAndWriteString(path, data string) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0777)
	defer f.Close()
	if err != nil {
		fmt.Println("OpenAndWriteString:", err)
		return
	}

	_, err = f.WriteString(data)
	if err != nil {
		fmt.Println("OpenAndWriteString:", err)
		return
	}
}
