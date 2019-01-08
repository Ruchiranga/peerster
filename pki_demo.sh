#!/bin/bash

go build
cd client
go build
cd ..

v=10

rm *.out

./peerster -UIPort=50000 -gossipAddr=127.0.0.1:5000 -peers=127.0.0.1:5001 -name=Alice -rtimer=$v > Alice.out &

sleep 5

./peerster -UIPort=50001 -gossipAddr=127.0.0.1:5001 -peers=127.0.0.1:5000,127.0.0.1:5002 -name=Eve -rtimer=$v > Eve.out &

sleep 5

./peerster -UIPort=50002 -gossipAddr=127.0.0.1:5002 -peers=127.0.0.1:5001 -name=Bob -rtimer=$v > Bob.out &

sleep 5
