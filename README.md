# Dicompot - A Digital Imaging and Communications in Medicine (DICOM) Honeypot

# Credit where credit is due

This project is built up on the great work done by GRAIL ( https://github.com/grailbio/go-netdicom)

# About

Dicompot is fully functional DICOM server with a twist. 

# Install

For an Debian based OS

- sudo apt install golang
- git clone https://github.com/nsmfoo/dicompot.git
- git clone https://github.com/gradienthealth/dicom.git
- cd dicompot 
 

# Run

go run server/server.go 

# Test

findscu -P -k PatientName="*" <IP> <PORT>
getscu  -P -k PatientName="*" <IP> <PORT>

Both commands are part of the DICOM Toolkit - DCMTK

# Known Issues

If the server instance dies with the message: "signal: killed" , try increasing the amount of avalible memory of the system and try again.

# ToDo

Enforce AET (So people can brute force away)
Auto generate meta data in DICOM files (for use in dicompot)
Block certain IP's (Geo based etc, amount of connections etc)
Code cleanup


# Note

I'm in no way a DICOM expert, if you find something strange... it probably is. Also this is the first GO code I ever touched so in the same way, if something looks strange, it probably is. That being said, any help, suggestions are more than welcome. 