## Peerster

##### Running Peerster and accessing the web client

* Navigate to `$GOPATH/src/github.com`
* Execute `go run Ruchiranga/Peerster/main.go -gossipAddr=... -peers=... -name=... -UIPort=...` along with the relevant arguments\
(Eg:- `go run Ruchiranga/Peerster/main.go -gossipAddr=127.0.0.1:5000 -peers=127.0.0.1:5001,127.0.0.1:5002 -name=jon_snow -UIPort=8080`)
* Open the web browser and access `http://localhost:8080/` (or what ever the UI port specified when running the server) to use the node's web client

##### Running tests

Before running `test_1_ring.sh` please make sure `simpleMode := true` is set in `client/main.go` main function. Similarly before running `test_2_ring.sh` it has to be set to `false`.