#!/usr/bin/env bash
#downloading file

go build
cd client
go build
cd ..

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'
DEBUG="false"

outputFiles=()
message_1=Weather_is_clear
message_2=No_clouds_really
message_3=Winter_is_coming
message_4=Let\'s_go_skiing
message_5=Is_anybody_here?

echo -e "${RED}###CHECK that DSDV updates happen${NC}"
failed="F"
pkill -f Peerster

./Peerster -UIPort=8081 -gossipAddr=127.0.0.1:5001 -name=A -peers=127.0.0.1:5002 > "A.out" &
./Peerster -UIPort=8082 -gossipAddr=127.0.0.1:5002 -name=B -peers=127.0.0.1:5003 > "B.out" &
./Peerster -UIPort=8083 -gossipAddr=127.0.0.1:5003 -name=C -peers= > "C.out" &


sleep 1

./client/client -UIPort=8081 -msg=${message_1}

./Peerster -UIPort=8084 -gossipAddr=127.0.0.1:5004 -name=D -peers=127.0.0.1:5002 > "D.out" &

sleep 1

./client/client -UIPort=8084 -msg=${message_2}

sleep 10

pkill -f Peerster


sleep 2

if !(grep -Eq "DSDV A 127.0.0.1:5001" "B.out") ; then
    echo -e "A in B"
	failed="T"
fi

if !(grep -Eq "DSDV A 127.0.0.1:5002" "C.out") ; then
    echo -e "A in C"
	failed="T"
fi

if !(grep -Eq "DSDV D 127.0.0.1:5004" "B.out") ; then
    echo -e "D in B"
	failed="T"
fi

if !(grep -Eq "DSDV D 127.0.0.1:5002" "C.out") ; then
    echo -e "D in C"
	failed="T"
fi

if [[ "$failed" == "T" ]] ; then
	echo -e "${RED}***FAILED***${NC}"
else
	echo -e "${GREEN}***PASSED***${NC}"
fi

sleep 2

echo -e "${RED}###CHECK that nodes learn next-hops with rtimer${NC}"
failed="F"


UIPort=8080
gossipPort=5000
name='A'

for i in `seq 1 10`;
do
	outFileName="$name.out"
	peerPort=$((($gossipPort+1)%10+5000))
	peer="127.0.0.1:$peerPort"
	gossipAddr="127.0.0.1:$gossipPort"
	./Peerster -UIPort=$UIPort -gossipAddr=$gossipAddr -name=$name -peers=$peer -rtimer=5 > $outFileName &
	outputFiles+=("$outFileName")
	if [[ "$DEBUG" == "true" ]] ; then
		echo "$name running at UIPort $UIPort and gossipPort $gossipPort"
	fi
	UIPort=$(($UIPort+1))
	gossipPort=$(($gossipPort+1))
	name=$(echo "$name" | tr "A-Y" "B-Z")
done

sleep 2

UIPort=8080
gossipPort=5000
name='A'

for i in `seq 1 10`;
do
	outFileName="$name.out"
	originName='A'
	for j in `seq 1 10`;
    do
    if [[ "$originName" != "$name" ]]; then
        if !(grep -q "DSDV $originName" ${outFileName}) ; then
            failed="T"
        fi
    fi
    originName=$(echo "$originName" | tr "A-Y" "B-Z")
    done
	gossipAddr="127.0.0.1:$gossipPort"


	name=$(echo "$name" | tr "A-Y" "B-Z")
done


if [[ "$failed" == "T" ]] ; then
	echo -e "${RED}***FAILED***${NC}"
else
	echo -e "${GREEN}***PASSED***${NC}"
fi

echo -e "${RED}###CHECK correct delivery of Private Messages${NC}"

./client/client -UIPort=8080 -dest=J  -msg=${message_1}
./client/client -UIPort=8080 -dest=H  -msg=${message_2}
./client/client -UIPort=8080 -dest=B  -msg=${message_3}
./client/client -UIPort=8083 -dest=C  -msg=${message_4}
./client/client -UIPort=8081 -dest=I  -msg=${message_5}

sleep 2

failed="F"

if !(grep -q "PRIVATE origin A hop-limit 2 contents $message_1" "J.out") && !(grep -q "PRIVATE origin A hop-limit 10 contents $message_1" "J.out") ; then
	failed="T"
fi

if !(grep -q "PRIVATE origin A hop-limit 4 contents $message_2" "H.out") && !(grep -q "PRIVATE origin A hop-limit 8 contents $message_2" "H.out") ; then
	failed="T"
fi

if !(grep -q "PRIVATE origin A hop-limit 10 contents $message_3" "B.out") && !(grep -q "PRIVATE origin A hop-limit 2 contents $message_3" "B.out") ; then
	failed="T"
fi

if !(grep -q "PRIVATE origin D hop-limit 10 contents $message_4" "C.out") && !(grep -q "PRIVATE origin D hop-limit 2 contents $message_4" "C.out") ; then
	failed="T"
fi

if !(grep -q "PRIVATE origin B hop-limit 4 contents $message_5" "I.out") && !(grep -q "PRIVATE origin B hop-limit 8 contents $message_5" "I.out") ; then
	failed="T"
fi

if [[ "$failed" == "T" ]] ; then
	echo -e "${RED}***FAILED***${NC}"
else
	echo -e "${GREEN}***PASSED***${NC}"
fi

echo -e "${RED}###CHECK file index functionality${NC}"
failed="F"

testFile_1=test_ring_3_1.txt
testFile_2=test_ring_3_2.txt
dummy_text="Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
Donec a tellus et diam interdum pretium. Suspendisse vulputate mi dui, et sollicitudin nunc vehicula a.
Cras ornare, dui in gravida maximus, purus odio bibendum lectus, nec consequat magna nisl vel massa. Cras et lectus
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
scelerisque, iaculis ligula nec, lobortis justo. Nam finibus leo a pharetra tempus. Curabitur gravida, justo sed congue
euismod, dui lectus posuere nisl, at dapibus lorem ante eget massa. Mauris auctor pellentesque aliquam. Nunc faucibus
neque fermentum tincidunt viverra. Ut viverra sapien ac ante placerat commodo. Aliquam eu nulla pharetra, cursus nibh
commodo, venenatis velit. Fusce sagittis sit amet lorem eu sagittis. Mauris iaculis eu sapien in rhoncus. Phasellus
commodo odio in condimentum consequat. Praesent convallis molestie augue sit amet blandit. Nulla libero risus, porttitor
varius eleifend at, eleifend eu ipsum. Pellentesque vel quam ultricies, egestas lorem et, mollis nibh. Quisque lobortis
pulvinar augue, vitae consectetur justo facilisis scelerisque. Aliquam tristique, magna vitae fringilla dapibus, nulla
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
quam vehicula tortor, non euismod odio lorem quis arcu. Integer tincidunt eleifend volutpat. Pellentesque habitant morbi
tristique senectus et netus et malesuada fames ac turpis egestas. Morbi et porta libero. Aliquam a tincidunt eros. Nunc
ultrices feugiat volutpat. Aenean vitae tempor ante. Phasellus ut vestibulum ipsum. Vivamus aliquam lacus rutrum neque
tincidunt bibendum. Donec diam risus, bibendum nec aliquet vitae, consequat venenatis purus. Praesent eu vestibulum mi.
"
metahash="84c2c909d3f8085ae2d3290d9ecceb7a176d6d9f893c34d5c45b553a74a49ce1"

touch ./_SharedFiles/${testFile_1}
echo $dummy_text > ./_SharedFiles/${testFile_1}
echo $dummy_text >> ./_SharedFiles/${testFile_1}

touch ./_SharedFiles/${testFile_2}
echo $dummy_text > ./_SharedFiles/${testFile_2}
echo $dummy_text >> ./_SharedFiles/${testFile_2}

./client/client -UIPort=8080 -file=${testFile_1}
./client/client -UIPort=8080 -file=${testFile_2}

sleep 2

if !(grep -q "INDEXED file $testFile_1 metahash $metahash" "A.out"); then
	failed="T"
fi

if !(grep -q "INDEXED file $testFile_2 metahash $metahash" "A.out"); then
	failed="T"
fi

if [[ "$failed" == "T" ]] ; then
	echo -e "${RED}***FAILED***${NC}"
else
	echo -e "${GREEN}***PASSED***${NC}"
fi

echo -e "${RED}###CHECK file download functionality${NC}"
failed="F"

down_file_I=testfile_1_I.txt
down_file_F=testfile_1_F.txt
down_file_B=testfile_1_B.txt
down_file_G=testfile_1_G.txt
down_file_H=testfile_1_H.txt

./client/client -UIPort=8088 -file=${down_file_I} -dest=B -request=$metahash &# I <- B
./client/client -UIPort=8085 -file=${down_file_F} -dest=A -request=$metahash &# F <- A
./client/client -UIPort=8081 -file=${down_file_B} -dest=G -request=$metahash &# B <- G
./client/client -UIPort=8086 -file=${down_file_G} -dest=A -request=$metahash &# G <- A
./client/client -UIPort=8087 -file=${down_file_H} -dest=A -request=$metahash &# H <- A

sleep 15
pkill -f Peerster

if !(grep -Eq "DOWNLOADING metafile of $down_file_I from B" "I.out"); then
    cat
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_I chunk 1 from B" "I.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_I chunk 2 from B" "I.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_I chunk 3 from B" "I.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_I chunk 4 from B" "I.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_I chunk 5 from B" "I.out"); then
	failed="T"
fi

if !(grep -Eq "RECONSTRUCTED file $down_file_I" "I.out"); then
	failed="T"
fi



if !(grep -Eq "DOWNLOADING metafile of $down_file_F from A" "F.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_F chunk 1 from A" "F.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_F chunk 2 from A" "F.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_F chunk 3 from A" "F.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_F chunk 4 from A" "F.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_F chunk 5 from A" "F.out"); then
	failed="T"
fi

if !(grep -Eq "RECONSTRUCTED file $down_file_F" "F.out"); then
	failed="T"
fi



if !(grep -Eq "DOWNLOADING metafile of $down_file_B from G" "B.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_B chunk 1 from G" "B.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_B chunk 2 from G" "B.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_B chunk 3 from G" "B.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_B chunk 4 from G" "B.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_B chunk 5 from G" "B.out"); then
	failed="T"
fi

if !(grep -Eq "RECONSTRUCTED file $down_file_B" "B.out"); then
	failed="T"
fi




if !(grep -Eq "DOWNLOADING metafile of $down_file_G from A" "G.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_G chunk 1 from A" "G.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_G chunk 2 from A" "G.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_G chunk 3 from A" "G.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_G chunk 4 from A" "G.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_G chunk 5 from A" "G.out"); then
	failed="T"
fi

if !(grep -Eq "RECONSTRUCTED file $down_file_G" "G.out"); then
	failed="T"
fi



if !(grep -Eq "DOWNLOADING metafile of $down_file_H from A" "H.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_H chunk 1 from A" "H.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_H chunk 2 from A" "H.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_H chunk 3 from A" "H.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_H chunk 4 from A" "H.out"); then
	failed="T"
fi

if !(grep -Eq "DOWNLOADING $down_file_H chunk 5 from A" "H.out"); then
	failed="T"
fi

if !(grep -Eq "RECONSTRUCTED file $down_file_H" "H.out"); then
	failed="T"
fi


if [[ "$failed" == "T" ]] ; then
	echo -e "${RED}***FAILED***${NC}"
else
	echo -e "${GREEN}***PASSED***${NC}"
fi

sleep 5

echo -e "${RED}###CHECK that private messages do not travel after hop-limit exceeded${NC}"
failed="F"


UIPort=8080
gossipPort=5000
name='A'

for i in `seq 1 12`;
do
	outFileName="$name.out"
	peerPort=$(($gossipPort+1))
	peer="127.0.0.1:$peerPort"
	gossipAddr="127.0.0.1:$gossipPort"
	./Peerster -UIPort=$UIPort -gossipAddr=$gossipAddr -name=$name -peers=$peer -rtimer=5 > $outFileName &
	outputFiles+=("$outFileName")
	if [[ "$DEBUG" == "true" ]] ; then
		echo "$name running at UIPort $UIPort and gossipPort $gossipPort"
	fi
	UIPort=$(($UIPort+1))
	gossipPort=$(($gossipPort+1))
	name=$(echo "$name" | tr "A-Y" "B-Z")
done

sleep 2

./client/client -UIPort=8080 -dest=L -msg=${message_1}

sleep 2
pkill -f Peerster

if (grep -q "PRIVATE origin A" "L.out"); then
	failed="T"
fi

if [[ "$failed" == "T" ]] ; then
	echo -e "${RED}***FAILED***${NC}"
else
	echo -e "${GREEN}***PASSED***${NC}"
fi