#!/bin/bash

go build
cd client
go build
cd ..

v=10

rm *.out

./peerster -UIPort=12000 -gossipAddr=127.0.0.1:5000 -peers=127.0.0.1:5001 -name=A -rtimer=$v > peersterA.out &

sleep 20

./peerster -UIPort=12001 -gossipAddr=127.0.0.1:5001 -peers=127.0.0.1:5000 -name=B -rtimer=$v > peersterB.out &

sleep 10

./client/client -UIPort=12001 -msg=test -dest=A -secure

sleep 5

pkill -f peerster
rm *.key

T=FAILURE

if (grep -q "FOUND KEY FROM BOOTSTRAP A" "peersterB.out") && (grep -q "ENC PRIVATE origin B.*contents test" "peersterA.out"); then
	T=SUCCESS
fi

echo $T