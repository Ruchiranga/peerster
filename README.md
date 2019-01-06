# Peer to Peer Audio Streaming Application based on Peerster

Aims to create a decentralized audio streaming application by building on the current implementation of Peerster.
Allows users to save storage by streaming files on demand instead of downloading them.
User privacy ensured with cryptographic tools.
A decentralized storage mechanism is used to ensure efficient packet streaming

### Prerequisites

There are two prerequisites, FFmpeg and LAME MP3 Encoder, the instructions to install them are below:

##### Installing FFmpeg for transcoding audio files
###### For Windows

* Download the compiled binaries from https://ffmpeg.zeranoe.com/builds/win64/static/ffmpeg-20190104-5faa1b8-win64-static.zip
* Unzip it to the folder of your choice
* Open a command prompt with admin rights.
* Run the following command (with the correct path):
```bash
setx /M PATH "path\to\ffmpeg\bin;%PATH%"
```

###### For Mac

* Install Homebrew by running the following command:
```bash
ruby -e "$(curl -fsSL https://raw.github.com/Homebrew/homebrew/go/install)"
```
* Run the following command to install FFmpeg:
```bash
brew install ffmpeg --with-tools
```

###### For Linux 

* Run the following command:
```bash
sudo apt-get install ffmpeg
```

##### Installing LAME MP3 Encoder as the library for encoding to the mp3 format
###### For Windows

* Download the following file and install it: https://lame.buanzo.org/Lame_v3.99.3_for_Windows.exe

###### For Mac

* Download the following file and install it: https://lame.buanzo.org/Lame_Library_v3.99.5_for_Audacity_on_macOS.dmg

###### For Linux 

* Run the following command:
```bash
sudo apt-get install lame
```

### Running Peerster and accessing the web client

* Navigate to `src/github.com/Ruchiranga/Peerster`
* Execute `go build`
* Execute `./Peerster -gossipAddr=... -peers=... -UIPort=... -rtimer=...` along with the relevant arguments\
(Eg:- `./Peerster -gossipAddr=127.0.0.1:5000 -peers=127.0.0.1:5001,127.0.0.1:5002 -UIPort=8080 -rtimer=5`)
* Open the web browser and access `http://localhost:8080/` (or whatever the UI port specified when running the server) 
to use the node's web client

##### Note

Please note that the `-name` parameter is not passed in when running `./Peerster` command since it now generates a name on its own to 
support decentralized storage functionality.

It is also assumed that the `go build` and `./Peerster` commands are executed being in `/src/github.com/Ruchiranga/Peerster` 
since the paths to the static web content are specified relative to that directory. If the commands are executed while 
being in a different location, web client will not work.

### Command line arguments description

- **UIPort** string
    Port for the UI client (default "8080")
    **On your browser you should enter: localhost:8080 or the port number you passed as a parameter**

- **gossipAddr** string
	ip:port for the gossiper (default "127.0.0.1:5000")
	
- **peers** string
	Comma separated list of peers of the form ip:port

- **rtimer** int
	Route rumors sending period in seconds, 0 to disable sending of route rumors (default 0)

- **simple** boolean
	Run Gossiper in simple broadcast mode (default false)

##### Secure private message
The secure private message feature is available on the CLI and GUI. In both cases, a user should wait some time when joining the network before sending secure private message, this is to make sure its key gets on the blockchain and that it sees the keys from other nodes.

###### CLI: 
Sending a secure private message on the CLI is similar to sending a regular private message, the only difference is the presence of an additional `secure` flag:
`./client -UIPort=<UIPort> -msg=<content> -dest=<Destination name> -secure`

For example:
`./client -UIPort=12000 -msg=hello -dest=Bob -secure`

###### GUI:
Sending a secure private message on the GUI is done by clicking on the `Send PM` button.

### Running tests

By default `client/main.go` has `simpleMode := false` set so that `test_2_ring.sh` can be run straight away. Before 
running `test_1_ring.sh` please make sure `simpleMode := true` is set in `client/main.go` main function. For other test
files this change shouldn't be necessary.
