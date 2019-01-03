#!/bin/bash

go build
cd client
go build
cd ..

v=10

rm *.out

./peerster -UIPort=12000 -gossipAddr=127.0.0.1:5000 -peers=127.0.0.1:5001 -name=A -rtimer=$v > peersterA.out &
./peerster -UIPort=12001 -gossipAddr=127.0.0.1:5001 -peers=127.0.0.1:5000 -name=B -rtimer=$v > peersterB.out &

sleep 10

pkill -f peerster
rm *.key

T=FAILURE

if (((grep -q "FOUND KEY A" "peersterA.out") && (grep -q "FOUND KEY B" "peersterA.out")) || ((grep -q "FOUND KEY A" "peersterB.out") && (grep -q "FOUND KEY B" "peersterB.out"))); then
#if ((grep -q "FOUND KEY" "peersterA.out") || (grep -q "FOUND KEY" "peersterB.out")); then
	T=SUCCESS
fi

echo $T