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
	keyWords := flag.String("keywords", "", "search for files having these key words")
	budget := flag.Int("budget", -1, "Search budget")
	secure := flag.Bool("secure", false, "run gossiper with security features")
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
		} else if *destination != "" {
			if *secure {
				content = fmt.Sprintf(`{"encprivate": {"temp": "%s", "destination": "%s"}}`, *message, *destination)
			} else {
				content = fmt.Sprintf(`{"private": {"text": "%s", "destination": "%s"}}`, *message, *destination)
			}
		} else {
			content = fmt.Sprintf(`{"rumor": {"text": "%s"}}`, *message)
		}
	} else if *file != "" {
		if *requestHash != "" {
			requestType = "GET"
			if *destination != "" {
				url = fmt.Sprintf("%s:%s/file?fileName=%s&destination=%s&metaHash=%s",
					"http://localhost", *uiPort, *file, *destination, *requestHash)
			} else {
				url = fmt.Sprintf("%s:%s/file?fileName=%s&metaHash=%s",
					"http://localhost", *uiPort, *file, *requestHash)
			}
		} else {
			requestType = "POST"
			url = fmt.Sprintf("%s:%s/file", "http://localhost", *uiPort)
			content = *file
		}
	} else if *keyWords != "" {
		requestType = "GET"
		if *budget > 0 {
			url = fmt.Sprintf("%s:%s/search?keywords=%s&budget=%d", "http://localhost", *uiPort, *keyWords, *budget)
		} else {
			url = fmt.Sprintf("%s:%s/search?keywords=%s", "http://localhost", *uiPort, *keyWords)
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
