#!/bin/bash

go build
cd client
go build
cd ..

v=10

rm *.out

./peerster -UIPort=12000 -gossipAddr=127.0.0.1:5000 -name=A -peers=127.0.0.1:5001,127.0.0.1:5014  -rtimer=$v > peersterA.out &
./peerster -UIPort=12001 -gossipAddr=127.0.0.1:5001 -name=B -peers=127.0.0.1:5000,127.0.0.1:5002  -rtimer=$v > peersterB.out &
./peerster -UIPort=12002 -gossipAddr=127.0.0.1:5002 -name=C -peers=127.0.0.1:5001,127.0.0.1:5003  -rtimer=$v > peersterC.out &
./peerster -UIPort=12003 -gossipAddr=127.0.0.1:5003 -name=D -peers=127.0.0.1:5002,127.0.0.1:5004  -rtimer=$v > peersterD.out &
./peerster -UIPort=12004 -gossipAddr=127.0.0.1:5004 -name=E -peers=127.0.0.1:5003,127.0.0.1:5005  -rtimer=$v > peersterE.out &
./peerster -UIPort=12005 -gossipAddr=127.0.0.1:5005 -name=F -peers=127.0.0.1:5004,127.0.0.1:5006  -rtimer=$v > peersterF.out &
./peerster -UIPort=12006 -gossipAddr=127.0.0.1:5006 -name=G -peers=127.0.0.1:5005,127.0.0.1:5007  -rtimer=$v > peersterG.out &
./peerster -UIPort=12007 -gossipAddr=127.0.0.1:5007 -name=H -peers=127.0.0.1:5006,127.0.0.1:5008  -rtimer=$v > peersterH.out &
./peerster -UIPort=12008 -gossipAddr=127.0.0.1:5008 -name=I -peers=127.0.0.1:5007,127.0.0.1:5009  -rtimer=$v > peersterI.out &
./peerster -UIPort=12009 -gossipAddr=127.0.0.1:5009 -name=J -peers=127.0.0.1:5008,127.0.0.1:5010  -rtimer=$v > peersterJ.out &
./peerster -UIPort=12010 -gossipAddr=127.0.0.1:5010 -name=K -peers=127.0.0.1:5009,127.0.0.1:5011  -rtimer=$v > peersterK.out &
./peerster -UIPort=12011 -gossipAddr=127.0.0.1:5011 -name=L -peers=127.0.0.1:5010,127.0.0.1:5012  -rtimer=$v > peersterL.out &
./peerster -UIPort=12012 -gossipAddr=127.0.0.1:5012 -name=M -peers=127.0.0.1:5011,127.0.0.1:5013  -rtimer=$v > peersterM.out &
./peerster -UIPort=12013 -gossipAddr=127.0.0.1:5013 -name=N -peers=127.0.0.1:5012,127.0.0.1:5014  -rtimer=$v > peersterN.out &
./peerster -UIPort=12014 -gossipAddr=127.0.0.1:5014 -name=O -peers=127.0.0.1:5013,127.0.0.1:5000  -rtimer=$v > peersterO.out &



sleep 60

./client/client -UIPort=12000 -msg=test -dest=F -secure

sleep 10

pkill -f peerster
rm *.key

T=FAILURE

#if (((grep -q "FOUND KEY A" "peersterA.out") && (grep -q "FOUND KEY F" "peersterA.out")) || ((grep -q "FOUND KEY A" "peersterF.out") && (grep -q "FOUND KEY J" "peersterJ.out"))) && (grep -q "ENC PRIVATE origin A.*contents test" "peersterJ.out"); then
#if (((grep -q "FOUND KEY A" "peersterA.out") && (grep -q "FOUND KEY F" "peersterA.out")) && ((grep -q "FOUND KEY A" "peersterF.out") && (grep -q "FOUND KEY F" "peersterF.out"))); then
if (grep -q "ENC PRIVATE origin A.*contents test" "peersterF.out"); then
	T=SUCCESS
fi

echo $T