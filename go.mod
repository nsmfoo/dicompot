module github.com/grailbio/go-netdicom

go 1.14

require (
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/grailbio/go-dicom v0.0.0
	github.com/mattn/go-colorable v0.1.6
	github.com/sirupsen/logrus v1.6.0
	github.com/snowzach/rotatefilehook v0.0.0-20180327172521-2f64f265f58c
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
)

replace github.com/grailbio/go-dicom => ../go-dicom

replace github.com/grailbio/go-netdicom => ../go-netdicom

replace github.com/golang/lint => ../../golang/lint
