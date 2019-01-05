#!/usr/bin/env bash

declare -a AddressList
declare -a LiveList

nodeCount=10

gossipPort=5000

for i in `seq 1 $nodeCount`;
do
	gossipAddr="127.0.0.1:$gossipPort"
	AddressList+=(${gossipAddr})
	gossipPort=$(($gossipPort+1))
done


UIPort=50000
gossipPort=5000

for i in `seq 1 $nodeCount`;
do
    outFileName="$gossipPort.out"

    peersString=""
    if [ ${#LiveList[@]} -gt 0 ]
    then
#        peerCount=$(jot -r 1 0 4)
        peerCount="$(python rand.py 4)"

        liveListLimit=$((${#LiveList[@]}-1))
        actualLimit=10
        if [ $liveListLimit -lt 10 ]
        then
        actualLimit=$liveListLimit
        fi

        # Let every node have one random neighbour out of all live nodes
#        randFirstIndex=$(jot -r 1 0 "${liveListLimit}")
        randFirstIndex="$(python rand.py "${liveListLimit}")"

        peersString=${LiveList[$randFirstIndex]}
        for j in $(seq 1 $peerCount);
        do
        # All the rest of neighbours will be from 5000 - 5010
#        randIndex=$(jot -r 1 0 "${liveListLimit}")
        randIndex="$(python rand.py "${liveListLimit}")"
        address=${LiveList[$randIndex]}
        peersString="$peersString,$address"
        done
        printf '%s\n' $peersString;
    fi




	gossipAddr="127.0.0.1:$gossipPort"
	./Peerster -UIPort=$UIPort -gossipAddr=$gossipAddr -rtimer=5 -peers=$peersString > "logs/$outFileName" &
	LiveList+=(${gossipAddr})
	UIPort=$(($UIPort+1))
	gossipPort=$(($gossipPort+1))

    sleepTime=$(($((${#LiveList[@]}/10+1)) * 5))
    printf 'sleeping %d\n' $sleepTime;
	sleep $sleepTime
#    read
done

#sleep 300
read
pkill -f Peerster
#num1="$(($num1+$num2))"
#
#${#LiveList[@]}
#
#$(($peerCount>${#LiveList[@]}?$JOBS:4))