## Peerster

##### Running Peerster and accessing the web client

* Navigate to `/src/github.com/Ruchiranga/Peerster`
* Execute `go build`
* Execute `./Peerster -gossipAddr=... -peers=... -name=... -UIPort=...` along with the relevant arguments\
(Eg:- `./Peerster -gossipAddr=127.0.0.1:5000 -peers=127.0.0.1:5001,127.0.0.1:5002 -name=jon_snow -UIPort=8080`)
* Open the web browser and access `http://localhost:8080/` (or whatever the UI port specified when running the server) 
to use the node's web client

###### Note

It is assumed that the `go build` and `./Peerster` commands are executed being in `/src/github.com/Ruchiranga/Peerster` 
since the paths to the static web content are specified relative to that directory. If the commands are executed while 
being in a different location, web client will not work.

##### Running tests

By default `client/main.go` has `simpleMode := false` set so that `test_2_ring.sh` can be run straight away. Before 
running `test_1_ring.sh` please make sure `simpleMode := true` is set in `client/main.go` main function.