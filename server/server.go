package main

import (
	"flag"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/grailbio/go-dicom"
	"github.com/mattn/go-colorable"
	"github.com/nsmfoo/dicompot"
	"github.com/nsmfoo/dicompot/dimse"
	"github.com/sirupsen/logrus"
	"github.com/snowzach/rotatefilehook"
)

var (
	portFlag = flag.String("port", "11112", "TCP port to listen to")
	ipFlag   = flag.String("ip", "127.0.0.1", "IP address to listen to")
	aeFlag   = flag.String("ae", "radiant", "AE title of this server")
	dirFlag  = flag.String("dir", ".", "Picture directory")
	logFlag  = flag.String("log", "dicompot.log", "logfile")
)

func logInit() {
	var logLevel = logrus.InfoLevel
	rotateFileHook, err := rotatefilehook.NewRotateFileHook(rotatefilehook.RotateFileConfig{
		Filename:   *logFlag,
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     7,
		Level:      logLevel,
		Formatter: &logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		},
	})

	if err != nil {
		logrus.Fatalf("Failed to initialize file rotate hook: %v", err)
	}

	logrus.SetOutput(colorable.NewColorableStdout())
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	logrus.AddHook(rotateFileHook)
}

type server struct {
	mu *sync.Mutex

	// Set of dicom files the server manages. Keys are file paths.
	datasets map[string]*dicom.DataSet
}

// Represents a match.
type filterMatch struct {
	path  string           // DICOM path name
	elems []*dicom.Element // Elements within "ds" that match the filter
}

// "filters" are matching conditions specified in C-{FIND,GET,MOVE}. This
// function returns the list of datasets and their elements that match filters.
func (ss *server) findMatchingFiles(filters []*dicom.Element) ([]filterMatch, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	var matches []filterMatch
	sum := 0
	for path, ds := range ss.datasets {
		allMatched := true
		match := filterMatch{path: path}
		for _, filter := range filters {
			ok, elem, err := dicom.Query(ds, filter)
			if err != nil {
				return matches, err
			}
			if !ok {
				s := strings.Split(filter.String(), " ")
				re := regexp.MustCompile(`\[(.*)\]`)
				matche1 := re.FindStringSubmatch(s[1])
				matche2 := re.FindStringSubmatch(s[4])
				if sum < 1 {
					logrus.WithFields(logrus.Fields{
						"Type": matche1[1],
						"Term": matche2[1],
					}).Info("C-FIND Search")
					sum++
				}
				allMatched = false
				break
			}
			if elem != nil {
				match.elems = append(match.elems, elem)
			} else {
				elem, err := dicom.NewElement(filter.Tag)
				if err != nil {
					log.Println(err)
					return matches, err
				}
				match.elems = append(match.elems, elem)
			}
		}
		if allMatched {
			if len(match.elems) == 0 {
				panic(match)
			}
			matches = append(matches, match)
		}
	}
	return matches, nil
}

func (ss *server) onCFind(
	transferSyntaxUID string,
	sopClassUID string,
	filters []*dicom.Element,
	ch chan dicompot.CFindResult) {
	logrus.WithFields(logrus.Fields{
		"Command": "C-FIND",
	}).Info("Received")
	matches, err := ss.findMatchingFiles(filters)
	logrus.WithFields(logrus.Fields{
		"Matches": len(matches),
	}).Warn("C-FIND Search result")
	if err != nil {
		ch <- dicompot.CFindResult{Err: err}
	} else {
		for _, match := range matches {
			ch <- dicompot.CFindResult{Elements: match.elems}
		}
	}
	close(ch)
}

func (ss *server) onCMoveOrCGet(
	transferSyntaxUID string,
	sopClassUID string,
	filters []*dicom.Element,
	ch chan dicompot.CMoveResult) {
	logrus.WithFields(logrus.Fields{
		"Command": "C-MOVE",
	}).Info("Received")
	matches, err := ss.findMatchingFiles(filters)
	if err != nil {
		ch <- dicompot.CMoveResult{Err: err}
	} else {
		for i, match := range matches {
			ds, err := dicom.ReadDataSetFromFile(match.path, dicom.ReadOptions{})
			resp := dicompot.CMoveResult{
				Remaining: len(matches) - i - 1,
				Path:      match.path,
			}
			if err != nil {
				resp.Err = err
			} else {
				resp.DataSet = ds
			}
			ch <- resp
		}
	}
	close(ch)
}

// Find DICOM files in or under "dir" and read its attributes.
func listDicomFiles(dir string) (map[string]*dicom.DataSet, error) {
	datasets := make(map[string]*dicom.DataSet)
	readFile := func(path string) {
		if _, ok := datasets[path]; ok {
			return
		}
		ds, err := dicom.ReadDataSetFromFile(path, dicom.ReadOptions{DropPixelData: true})
		if err != nil {
			log.Printf("%s: failed to parse dicom file: %v", path, err)
			return
		}
		datasets[path] = ds
	}

	walkCallback := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("%v: skip file: %v", path, err)
			return nil
		}
		if (info.Mode() & os.ModeDir) != 0 {
			// If a directory contains file "DICOMDIR", all the files in the directory are DICOM files.
			if _, err := os.Stat(filepath.Join(path, "DICOMDIR")); err != nil {
				return nil
			}
			subpaths, err := filepath.Glob(path + "/*")
			if err != nil {
				log.Printf("%v: glob: %v", path, err)
				return nil
			}
			for _, subpath := range subpaths {
				if !strings.HasSuffix(subpath, "DICOMDIR") {
					readFile(subpath)
				}
			}
			return nil
		}
		if strings.HasSuffix(path, ".dcm") {
			readFile(path)
		}
		return nil
	}
	if err := filepath.Walk(dir, walkCallback); err != nil {
		return nil, err
	}
	return datasets, nil
}

func canonicalizeHostPort(TcpPort string) string {
	if !strings.Contains(TcpPort, ":") {
		return ":" + TcpPort
	}
	return TcpPort
}

func canonicalizeHostIp(IpAdr string) string {
	if net.ParseIP(IpAdr) == nil {
		logrus.WithFields(logrus.Fields{
			"IP Address": strings.Replace(IpAdr, "\"", "", -1),
		}).Error("Invalid IP address, please try again")
		os.Exit(1)
	}
	return IpAdr
}

func main() {
	flag.Parse()
	logInit()
	port := canonicalizeHostPort(*portFlag)
	ip := canonicalizeHostIp(*ipFlag)
	hostAddress := ip + port
	datasets, err := listDicomFiles(*dirFlag)

	log.Printf(`
		██████╗ ██╗ ██████╗ ██████╗ ███╗   ███╗██████╗  ██████╗ ████████╗
		██╔══██╗██║██╔════╝██╔═══██╗████╗ ████║██╔══██╗██╔═══██╗╚══██╔══╝
		██║  ██║██║██║     ██║   ██║██╔████╔██║██████╔╝██║   ██║   ██║   
		██║  ██║██║██║     ██║   ██║██║╚██╔╝██║██╔═══╝ ██║   ██║   ██║   
		██████╔╝██║╚██████╗╚██████╔╝██║ ╚═╝ ██║██║     ╚██████╔╝   ██║   
		╚═════╝ ╚═╝ ╚═════╝ ╚═════╝ ╚═╝     ╚═╝╚═╝      ╚═════╝    ╚═╝  
		@nsmfoo - Mikael Keri
																	 
	`)
	log.Printf("-| Loaded %d images", len(datasets))
	ss := server{
		mu:       &sync.Mutex{},
		datasets: datasets,
	}
	log.Printf("-| Listening on: %s", hostAddress)

	params := dicompot.ServiceProviderParams{
		AETitle: *aeFlag,
		CEcho: func(connState dicompot.ConnectionState) dimse.Status {
			logrus.WithFields(logrus.Fields{
				"Command": "C-ECHO",
			}).Info("Received")

			return dimse.Success
		},
		CFind: func(connState dicompot.ConnectionState, transferSyntaxUID string, sopClassUID string,
			filter []*dicom.Element, ch chan dicompot.CFindResult) {
			ss.onCFind(transferSyntaxUID, sopClassUID, filter, ch)
		},
		CMove: func(connState dicompot.ConnectionState, transferSyntaxUID string, sopClassUID string,
			filter []*dicom.Element, ch chan dicompot.CMoveResult) {
			ss.onCMoveOrCGet(transferSyntaxUID, sopClassUID, filter, ch)
		},
		CGet: func(connState dicompot.ConnectionState, transferSyntaxUID string, sopClassUID string,
			filter []*dicom.Element, ch chan dicompot.CMoveResult) {
			ss.onCMoveOrCGet(transferSyntaxUID, sopClassUID, filter, ch)
		},
	}

	log.Printf("-| Local AE Title: %s", params.AETitle)
	log.Printf("-| Attacker log: %s", *logFlag)

	sp, err := dicompot.NewServiceProvider(params, hostAddress)
	if err != nil {
		panic(err)
	}
	sp.Run()
}
