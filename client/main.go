package main

import (
	"flag"
	"fmt"
)

func main() {
	uiPort := flag.String("UIPort", "10000", "Port for the UI client (default \"8080\"")
	message := flag.String("msg", "", "message to be sent")

	fmt.Println(*uiPort)
	fmt.Println(*message)
}