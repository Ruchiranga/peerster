#!/bin/bash

go build
cd client
go build
cd ..

v=10

rm *.out

./peerster -UIPort=12000 -gossipAddr=127.0.0.1:5000 -name=A -peers=127.0.0.1:5001 -rtimer=$v > peersterA.out &
./peerster -UIPort=12001 -gossipAddr=127.0.0.1:5001 -name=B -peers=127.0.0.1:5000 -rtimer=$v > peersterB.out &

sleep 20

./client/client -UIPort=12000 -msg=test -dest=B -secure

sleep 10

pkill -f peerster
rm *.key

T=FAILURE

if (((grep -q "FOUND KEY A" "peersterA.out") && (grep -q "FOUND KEY B" "peersterA.out")) || ((grep -q "FOUND KEY A" "peersterB.out") && (grep -q "FOUND KEY B" "peersterB.out"))) && (grep -q "ENC PRIVATE origin A.*contents test" "peersterB.out"); then
	T=SUCCESS
fi

echo $T