package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
)

func main() {
	uiPort := flag.String("UIPort", "8080", "Port for the UI client (default \"8080\"")
	message := flag.String("msg", "", "message to be sent")

	simpleMode := false

	flag.Parse()

	url := fmt.Sprintf("%s:%s/message", "http://localhost", *uiPort)
	var contentRumour string
	if simpleMode {
		contentRumour = fmt.Sprintf(`{"simple": {"contents": "%s"}}`, *message)
	} else {
		contentRumour = fmt.Sprintf(`{"rumor": {"text": "%s"}}`, *message)
	}

	var jsonStr = []byte(contentRumour)
	http.Post(url, "application/json", bytes.NewBuffer(jsonStr))
}
