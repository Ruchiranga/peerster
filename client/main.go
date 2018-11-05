package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
)

func main() {
	uiPort := flag.String("UIPort", "8080", "Port for the UI client (default \"8080\"")
	destination := flag.String("dest", "", "destination for the private message")
	message := flag.String("msg", "", "message to be sent")
	file := flag.String("file", "", "file to be indexed by the gossiper")
	requestHash := flag.String("request", "", "request a chunk or metafile of this hash")
	simpleMode := false

	flag.Parse()

	var url string
	var content string
	var requestType string

	if *message != "" {
		url = fmt.Sprintf("%s:%s/message", "http://localhost", *uiPort)
		requestType = "POST"
		if simpleMode {
			content = fmt.Sprintf(`{"simple": {"contents": "%s"}}`, *message)
		} else if *destination != "" && *message != "" {
			content = fmt.Sprintf(`{"private": {"text": "%s", "destination": "%s"}}`, *message, *destination)
		} else {
			content = fmt.Sprintf(`{"rumor": {"text": "%s"}}`, *message)
		}
	} else if *file != "" {
		if *requestHash != "" {
			requestType = "GET"
			url = fmt.Sprintf("%s:%s/file?fileName=%s&destination=%s&metaHash=%s", "http://localhost", *uiPort, *file, *destination, *requestHash)
		} else {
			requestType = "POST"
			url = fmt.Sprintf("%s:%s/file", "http://localhost", *uiPort)
			content = *file
		}
	}

	var jsonStr = []byte(content)
	switch requestType {
	case "GET":
		{
			http.Get(url)
		}
		break
	case "POST":
		{
			http.Post(url, "application/json", bytes.NewBuffer(jsonStr))
		}
	}

}
