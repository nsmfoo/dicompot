// This file defines ServiceProvider (i.e., a DICOM server).

package dicompot

import (
	"net"
	"regexp"
	"strings"

	dicom "github.com/grailbio/go-dicom"
	"github.com/grailbio/go-dicom/dicomio"
	"github.com/nsmfoo/dicompot/dimse"
	"github.com/sirupsen/logrus"
)

// CMoveResult is an object streamed by CMove implementation.
type CMoveResult struct {
	Remaining int // Number of files remaining to be sent. Set -1 if unknown.
	Err       error
	Path      string         // Path name of the DICOM file being copied. Used only for reporting errors.
	DataSet   *dicom.DataSet // Contents of the file.
}

func handleCStore(
	cb CStoreCallback,
	connState ConnectionState,
	c *dimse.CStoreRq, data []byte,
	cs *serviceCommandState) {
	status := dimse.Status{Status: dimse.StatusUnrecognizedOperation}

	if cb != nil {
		status = cb(
			connState,
			cs.context.transferSyntaxUID,
			c.AffectedSOPClassUID,
			c.AffectedSOPInstanceUID,
			data)
	}
	resp := &dimse.CStoreRsp{
		AffectedSOPClassUID:       c.AffectedSOPClassUID,
		MessageIDBeingRespondedTo: c.MessageID,
		CommandDataSetType:        dimse.CommandDataSetTypeNull,
		AffectedSOPInstanceUID:    c.AffectedSOPInstanceUID,
		Status:                    status,
	}
	cs.sendMessage(resp, nil)

	logrus.WithFields(logrus.Fields{
		"Type": "We don't like that",
		"ID":   cs.disp.label,
	}).Error("C-STORE received")
}

func handleCFind(
	params ServiceProviderParams,
	connState ConnectionState,
	c *dimse.CFindRq, data []byte,
	cs *serviceCommandState) {

	if params.CFind == nil {
		cs.sendMessage(&dimse.CFindRsp{
			AffectedSOPClassUID:       c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo: c.MessageID,
			CommandDataSetType:        dimse.CommandDataSetTypeNull,
			Status:                    dimse.Status{Status: dimse.StatusUnrecognizedOperation, ErrorComment: "No callback found for C-FIND"},
		}, nil)
		return
	}
	elems, err := readElementsInBytes(data, cs.context.transferSyntaxUID)
	if err != nil {
		cs.sendMessage(&dimse.CFindRsp{
			AffectedSOPClassUID:       c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo: c.MessageID,
			CommandDataSetType:        dimse.CommandDataSetTypeNull,
			Status:                    dimse.Status{Status: dimse.StatusUnrecognizedOperation, ErrorComment: err.Error()},
		}, nil)
		return
	}

	status := dimse.Status{Status: dimse.StatusSuccess}
	responseCh := make(chan CFindResult, 128)
	var sessionID string = cs.cm.label

	go func() {
		params.CFind(connState, cs.context.transferSyntaxUID, c.AffectedSOPClassUID, elems, sessionID, responseCh)
	}()
	for resp := range responseCh {
		if resp.Err != nil {
			status = dimse.Status{
				Status:       dimse.CFindUnableToProcess,
				ErrorComment: resp.Err.Error(),
			}
			break
		}
		payload, err := writeElementsToBytes(resp.Elements, cs.context.transferSyntaxUID)

		if err != nil {
			status = dimse.Status{
				Status:       dimse.CFindUnableToProcess,
				ErrorComment: err.Error(),
			}
			break
		}

		cs.sendMessage(&dimse.CFindRsp{
			AffectedSOPClassUID:       c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo: c.MessageID,
			CommandDataSetType:        dimse.CommandDataSetTypeNonNull,
			Status:                    dimse.Status{Status: dimse.StatusPending},
		}, payload)
	}

	logrus.WithFields(logrus.Fields{
		"Command": "C-FIND",
		"ID":      cs.cm.label,
	}).Info("Received")

	cs.sendMessage(&dimse.CFindRsp{
		AffectedSOPClassUID:       c.AffectedSOPClassUID,
		MessageIDBeingRespondedTo: c.MessageID,
		CommandDataSetType:        dimse.CommandDataSetTypeNull,
		Status:                    status}, nil)
	// Drain the responses in case of errors
	for range responseCh {
	}
}

func handleCMove(
	params ServiceProviderParams,
	connState ConnectionState,
	c *dimse.CMoveRq, data []byte,
	cs *serviceCommandState) {

	logrus.WithFields(logrus.Fields{
		"Command": "C-MOVE",
		"ID":      cs.cm.label,
	}).Info("Received")

	sendError := func(err error) {
		cs.sendMessage(&dimse.CMoveRsp{
			AffectedSOPClassUID:       c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo: c.MessageID,
			CommandDataSetType:        dimse.CommandDataSetTypeNull,
			Status:                    dimse.Status{Status: dimse.StatusUnrecognizedOperation, ErrorComment: err.Error()},
		}, nil)
	}
	if params.CMove == nil {
		cs.sendMessage(&dimse.CMoveRsp{
			AffectedSOPClassUID:       c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo: c.MessageID,
			CommandDataSetType:        dimse.CommandDataSetTypeNull,
			Status:                    dimse.Status{Status: dimse.StatusUnrecognizedOperation, ErrorComment: "No callback found for C-MOVE"},
		}, nil)
		return
	}
	elems, err := readElementsInBytes(data, cs.context.transferSyntaxUID)
	if err != nil {
		sendError(err)
		return
	}
	var sessionID string = cs.cm.label
	responseCh := make(chan CMoveResult, 128)
	go func() {
		params.CMove(connState, cs.context.transferSyntaxUID, c.AffectedSOPClassUID, elems, sessionID, responseCh)
	}()
	status := dimse.Status{Status: dimse.StatusSuccess}
	var numSuccesses, numFailures uint16
	for resp := range responseCh {
		if resp.Err != nil {
			status = dimse.Status{
				Status:       dimse.CFindUnableToProcess,
				ErrorComment: resp.Err.Error(),
			}
			break
		}

		cs.sendMessage(&dimse.CMoveRsp{
			AffectedSOPClassUID:            c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo:      c.MessageID,
			CommandDataSetType:             dimse.CommandDataSetTypeNull,
			NumberOfRemainingSuboperations: uint16(resp.Remaining),
			NumberOfCompletedSuboperations: numSuccesses,
			NumberOfFailedSuboperations:    numFailures,
			Status:                         dimse.Status{Status: dimse.StatusPending},
		}, nil)
	}
	cs.sendMessage(&dimse.CMoveRsp{
		AffectedSOPClassUID:            c.AffectedSOPClassUID,
		MessageIDBeingRespondedTo:      c.MessageID,
		CommandDataSetType:             dimse.CommandDataSetTypeNull,
		NumberOfCompletedSuboperations: numSuccesses,
		NumberOfFailedSuboperations:    numFailures,
		Status:                         status}, nil)
	// Drain the responses in case of errors
	for range responseCh {
	}
}

func handleCGet(
	params ServiceProviderParams,
	connState ConnectionState,

	c *dimse.CGetRq, data []byte, cs *serviceCommandState) {
	sendError := func(err error) {
		cs.sendMessage(&dimse.CGetRsp{
			AffectedSOPClassUID:       c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo: c.MessageID,
			CommandDataSetType:        dimse.CommandDataSetTypeNull,
			Status:                    dimse.Status{Status: dimse.StatusUnrecognizedOperation, ErrorComment: err.Error()},
		}, nil)
	}

	if params.CGet == nil {
		cs.sendMessage(&dimse.CGetRsp{
			AffectedSOPClassUID:       c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo: c.MessageID,
			CommandDataSetType:        dimse.CommandDataSetTypeNull,
			Status:                    dimse.Status{Status: dimse.StatusUnrecognizedOperation, ErrorComment: "No callback found for C-GET"},
		}, nil)
		return
	}
	elems, err := readElementsInBytes(data, cs.context.transferSyntaxUID)
	if err != nil {
		sendError(err)
		return
	}

	var sessionID string = cs.cm.label
	responseCh := make(chan CMoveResult, 128)
	go func() {
		params.CGet(connState, cs.context.transferSyntaxUID, c.AffectedSOPClassUID, elems, sessionID, responseCh)
	}()
	status := dimse.Status{Status: dimse.StatusSuccess}
	var numSuccesses, numFailures uint16
	for resp := range responseCh {
		if resp.Err != nil {
			status = dimse.Status{
				Status:       dimse.CFindUnableToProcess,
				ErrorComment: resp.Err.Error(),
			}
			break
		}
		subCs, err := cs.disp.newCommand(cs.cm, cs.context /*not used*/)
		if err != nil {
			status = dimse.Status{
				Status:       dimse.CFindUnableToProcess,
				ErrorComment: err.Error(),
			}
			break
		}

		err = runCStoreOnAssociation(subCs.upcallCh, subCs.disp.downcallCh, subCs.cm, subCs.messageID, resp.DataSet)
		if err != nil {
			numFailures++
		} else {
			numSuccesses++
		}
		cs.sendMessage(&dimse.CGetRsp{
			AffectedSOPClassUID:            c.AffectedSOPClassUID,
			MessageIDBeingRespondedTo:      c.MessageID,
			CommandDataSetType:             dimse.CommandDataSetTypeNull,
			NumberOfRemainingSuboperations: uint16(resp.Remaining),
			NumberOfCompletedSuboperations: numSuccesses,
			NumberOfFailedSuboperations:    numFailures,
			Status:                         dimse.Status{Status: dimse.StatusPending},
		}, nil)
		cs.disp.deleteCommand(subCs)
	}
	cs.sendMessage(&dimse.CGetRsp{
		AffectedSOPClassUID:            c.AffectedSOPClassUID,
		MessageIDBeingRespondedTo:      c.MessageID,
		CommandDataSetType:             dimse.CommandDataSetTypeNull,
		NumberOfCompletedSuboperations: numSuccesses,
		NumberOfFailedSuboperations:    numFailures,
		Status:                         status}, nil)

	logrus.WithFields(logrus.Fields{
		"Command": "C-GET",
		"Files":   numSuccesses,
		"ID":      cs.cm.label,
	}).Info("Received")

	// Drain the responses in case of errors
	for range responseCh {
	}
}

func handleCEcho(
	params ServiceProviderParams,
	connState ConnectionState,
	c *dimse.CEchoRq, data []byte,
	cs *serviceCommandState) {
	status := dimse.Status{Status: dimse.StatusUnrecognizedOperation}

	if params.CEcho != nil {
		status = params.CEcho(connState)
	}
	resp := &dimse.CEchoRsp{
		MessageIDBeingRespondedTo: c.MessageID,
		CommandDataSetType:        dimse.CommandDataSetTypeNull,
		Status:                    status,
	}

	logrus.WithFields(logrus.Fields{
		"Command": "C-ECHO",
		"ID":      cs.cm.label,
	}).Info("Received")

	cs.sendMessage(resp, nil)
}

// ServiceProviderParams defines parameters for ServiceProvider.
type ServiceProviderParams struct {
	// The application-entity title of the server. Must be nonempty
	AETitle string

	// Enforce AETitle, default accept any
	Enforce string

	// Names of remote AEs and their host:ports. Used only by C-MOVE. This
	// map should be nonempty iff the server supports CMove.
	RemoteAEs map[string]string

	// Called on C_ECHO request. If nil, a C-ECHO call will produce an error response.
	CEcho CEchoCallback

	// Called on C_FIND request.
	// If CFindCallback=nil, a C-FIND call will produce an error response.
	CFind CFindCallback

	// CMove is called on C_MOVE request.
	CMove CMoveCallback

	// CGet is called on C_GET request. The only difference between cmove
	// and cget is that cget uses the same connection to send images back to
	// the requester. Generally you shuold set the same function to CMove
	// and CGet.
	CGet CMoveCallback

	// If CStoreCallback=nil, a C-STORE call will produce an error response.
	CStore CStoreCallback
}

// DefaultMaxPDUSize is the the PDU size advertized.
const DefaultMaxPDUSize = 4 << 20

type CStoreCallback func(
	conn ConnectionState,
	transferSyntaxUID string,
	sopClassUID string,
	sopInstanceUID string,
	data []byte) dimse.Status

// CFindCallback implements a C-FIND handler
type CFindCallback func(
	conn ConnectionState,
	transferSyntaxUID string,
	sopClassUID string,
	filters []*dicom.Element,
	sessionID string,
	ch chan CFindResult)

// CMoveCallback implements C-MOVE or C-GET handle
type CMoveCallback func(
	conn ConnectionState,
	transferSyntaxUID string,
	sopClassUID string,
	filters []*dicom.Element,
	sessionID string,
	ch chan CMoveResult)

// ConnectionState informs session state to callbacks.
type ConnectionState struct {
}

// CEchoCallback implements C-ECHO callback.
type CEchoCallback func(conn ConnectionState) dimse.Status

// ServiceProvider encapsulates the state for DICOM server (provider).
type ServiceProvider struct {
	params   ServiceProviderParams
	listener net.Listener
	// Label is a unique string used in log messages to identify this provider.
	label string
}

func writeElementsToBytes(elems []*dicom.Element, transferSyntaxUID string) ([]byte, error) {
	dataEncoder := dicomio.NewBytesEncoderWithTransferSyntax(transferSyntaxUID)
	for _, elem := range elems {
		dicom.WriteElement(dataEncoder, elem)
	}
	if err := dataEncoder.Error(); err != nil {
		return nil, err
	}
	return dataEncoder.Bytes(), nil
}

func readElementsInBytes(data []byte, transferSyntaxUID string) ([]*dicom.Element, error) {
	decoder := dicomio.NewBytesDecoderWithTransferSyntax(data, transferSyntaxUID)
	var elems []*dicom.Element
	for !decoder.EOF() {
		elem := dicom.ReadElement(decoder, dicom.ReadOptions{})
		if decoder.Error() != nil {
			break
		}

		re := regexp.MustCompile(`\[([^\[\]]*)\]`)
		searchTerm := re.FindAllString(elem.String(), -1)

		searchTerm[0] = strings.Trim(searchTerm[0], "[")
		searchTerm[0] = strings.Trim(searchTerm[0], "]")
		searchTerm[1] = strings.Trim(searchTerm[1], "[")
		searchTerm[1] = strings.Trim(searchTerm[1], "]")

		if searchTerm[1] != "" && searchTerm[1] != "ISO_IR 100" && searchTerm[1] != "STUDY" {
			logrus.WithFields(logrus.Fields{
				"Type": searchTerm[0],
				"Term": searchTerm[1],
				"ID":   attackID,
			}).Info("C-FIND Search")
		}

		elems = append(elems, elem)
	}
	if decoder.Error() != nil {
		return nil, decoder.Error()
	}
	return elems, nil
}

// NewServiceProvider creates a new DICOM server object.
func NewServiceProvider(params ServiceProviderParams, port string) (*ServiceProvider, error) {
	sp := &ServiceProvider{
		params: params,
		label:  newUID(),
	}

	var err error
	sp.listener, err = net.Listen("tcp", port)

	if err != nil {
		return nil, err
	}
	return sp, nil
}

func getConnState(conn net.Conn) (cs ConnectionState) {
	return
}

var attackID string

// RunProviderForConn starts threads for running a DICOM server on "conn".
func RunProviderForConn(conn net.Conn, params ServiceProviderParams) {

	var clientAETitle = params.AETitle
	var enforce = params.Enforce

	upcallCh := make(chan upcallEvent, 128)

	label := newUID()
	disp := newServiceDispatcher(label)

	attackID = label

	RemoteAddress := conn.RemoteAddr()
	IPPort := strings.Split(RemoteAddress.String(), ":")
	logrus.WithFields(logrus.Fields{
		"IP":   IPPort[0],
		"Port": IPPort[1],
		"ID":   label,
	}).Warn("Connection from")

	disp.registerCallback(dimse.CommandFieldCStoreRq,
		func(msg dimse.Message, data []byte, cs *serviceCommandState) {
			handleCStore(params.CStore, getConnState(conn), msg.(*dimse.CStoreRq), data, cs)
		})
	disp.registerCallback(dimse.CommandFieldCFindRq,
		func(msg dimse.Message, data []byte, cs *serviceCommandState) {
			handleCFind(params, getConnState(conn), msg.(*dimse.CFindRq), data, cs)
		})

	disp.registerCallback(dimse.CommandFieldCMoveRq,
		func(msg dimse.Message, data []byte, cs *serviceCommandState) {
			handleCMove(params, getConnState(conn), msg.(*dimse.CMoveRq), data, cs)
		})
	disp.registerCallback(dimse.CommandFieldCGetRq,
		func(msg dimse.Message, data []byte, cs *serviceCommandState) {
			handleCGet(params, getConnState(conn), msg.(*dimse.CGetRq), data, cs)
		})
	disp.registerCallback(dimse.CommandFieldCEchoRq,
		func(msg dimse.Message, data []byte, cs *serviceCommandState) {
			handleCEcho(params, getConnState(conn), msg.(*dimse.CEchoRq), data, cs)
		})
	go runStateMachineForServiceProvider(conn, upcallCh, disp.downcallCh, label, clientAETitle, enforce)

	for event := range upcallCh {
		disp.handleEvent(event)
	}

	logrus.WithFields(logrus.Fields{
		"Status": "Finished",
		"ID":     label,
	}).Warn("Connection")
	disp.close()
}

// Run listens to incoming connections,
func (sp *ServiceProvider) Run() {

	for {
		conn, err := sp.listener.Accept()
		if err != nil {
			continue
		}
		go func() {

			RunProviderForConn(conn, sp.params)
		}()
	}
}

// ListenAddr returns the TCP address that the server is listening on
func (sp *ServiceProvider) ListenAddr() net.Addr {

	return sp.listener.Addr()
}
