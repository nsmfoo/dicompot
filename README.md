# Dicompot - A Digital Imaging and Communications in Medicine (DICOM) Honeypot

# Credit where credit is due

This project is built up on the great work done by GRAIL (https://github.com/grailbio/go-netdicom)

# About

- Dicompot is a fully functional DICOM server with a twist. 
- Please note: C-STORE attempts are blocked for your "protection", but logged. 

# Install
(Ubuntu 20.04 LTS)

- sudo apt install golang
- mkdir $HOME/go 
- cd $HOME/go
- go get github.com/nsmfoo/dicompot
- go get -d ./...
- go install -a -x github.com/nsmfoo/dicompot/server/

(Alpine)

- apk -U add go build-base g++ git
- mkdir -p /opt/go
- export GOPATH=/opt/go/
- cd /opt/go/
- git clone https://github.com/nsmfoo/dicompot.git
- cd dicompot
- go mod download
- go install -a -x github.com/nsmfoo/dicompot/server
- Binary (`server`) located in `/opt/go/bin`.

# Run

- cd $HOME/go/bin
- ./server 
- ./server -help, for the different options that is avalible
- The server will log to the console and also to a file called dicompot.log (JSON)
- Works well with screen, if you like to run it in the background

# Test

- findscu -P -k PatientName="*" IP PORT
- getscu -P -k PatientName="*" IP PORT

Both commands are part of the DICOM Toolkit - DCMTK

- Also tested with Horos (https://horosproject.org/) 

# Docker
## Build a Dicompot Docker Image from this repository
1. `git clone https://github.com/nsmfoo/dicompot.git`

2. `cd dicompot`

3. `docker build -t dicompot:latest .`

## Run the container
`docker run --rm --read-only --net="host" --name="dicompot" --detach --tty --interactive --publish=11112:11112 dicompot:latest`
### Docker Engine command-line arguments explained
* `--rm`: remove container after it stops
* `--read-only`: mount container as read-only
* `--net="host"`: set container network as `host`
* `--name="dicompot"`: set container name as `dicompot`
* `--detach`: run in the background *hint: use `docker attach dicompot` to re-attach STDIN/STDOUT back to the container*
* `--tty`: allocate a pseudo-tty for the container
* `--interactive`: make the container interactive

## View logs 
* `docker logs dicompot`

# Known Issues

If the server instance, terminates with the message: "signal: killed", try increasing the amount of avalible memory and try again.
(dmesg, should give you more information)

# ToDo

- ~~Enforce AET (So people can brute force away)~~
- Auto generate meta data in DICOM files (for use in dicompot)
- Block certain IP's (Geo based, amount of connections etc)
- Code cleanup


# Note

I'm in no way a DICOM expert, if you find something strange... it probably is. Also this is the first GO code I ever touched, so in the same way, if something looks strange, it probably is. That being said, any help, suggestions are more than welcome. 
