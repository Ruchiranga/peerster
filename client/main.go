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

	flag.Parse()

	url := fmt.Sprintf("%s:%s","http://localhost", *uiPort)
	//contentSimple := fmt.Sprintf(`{"simple": {"contents": "%s"}}`, *message)
	contentRumour := fmt.Sprintf(`{"rumour": {"text": "%s"}}`, *message)

	//fmt.Printf("URL: %s Message: %s", url, content)

	var jsonStr = []byte(contentRumour)
	http.Post(url, "application/json", bytes.NewBuffer(jsonStr))
}