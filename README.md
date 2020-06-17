# Dicompot - A Digital Imaging and Communications in Medicine (DICOM) Honeypot

# Credit where credit is due

This project is built up on the great work done by GRAIL (https://github.com/grailbio/go-netdicom)

# About

- Dicompot is a fully functional DICOM server with a twist. 
- Please note: C-STORE attempts are blocked for your "protection" but logged. 

# Install
(Ubuntu 20.04 LTS)

- sudo apt install golang
- mkdir $HOME/go 
- cd $HOME/go
- go get github.com/nsmfoo/dicompot
- go get -d ./...
- go install -a -x github.com/nsmfoo/dicompot/server/

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

- Also tested with Hooros (https://horosproject.org/) 

# Known Issues

If the server instance, terminates with the message: "signal: killed", try increasing the amount of avalible memory and try again.
(dmesg, should give you more information)

# ToDo

- Enforce AET (So people can brute force away)
- Auto generate meta data in DICOM files (for use in dicompot)
- Block certain IP's (Geo based, amount of connections etc)
- Code cleanup


# Note

I'm in no way a DICOM expert, if you find something strange... it probably is. Also this is the first GO code I ever touched, so in the same way, if something looks strange, it probably is. That being said, any help, suggestions are more than welcome. 