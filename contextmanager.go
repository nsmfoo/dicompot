package dicompot

import (
	"fmt"

	"github.com/grailbio/go-dicom"
	"github.com/grailbio/go-dicom/dicomuid"
	"github.com/nsmfoo/dicompot/pdu"
	"github.com/sirupsen/logrus"
)

type contextManagerEntry struct {
	contextID         byte
	abstractSyntaxUID string
	transferSyntaxUID string
	result            pdu.PresentationContextResult // was this mapping accepted by the server?
}

// contextManager manages mappings between a contextID and the corresponding
// abstract-syntax UID (aka SOP).  UID is of form "1.2.840.10008.5.1.4.1.1.1.2".
// UIDs are static and global.
type contextManager struct {
	label string // for diagnostics only.

	// The two maps are inverses of each other.
	contextIDToAbstractSyntaxNameMap map[byte]*contextManagerEntry
	abstractSyntaxNameToContextIDMap map[string]*contextManagerEntry

	// Info about the the other side of the communication, gleaned from
	// A-ASSOCIATE-* pdu.
	peerMaxPDUSize int
	// UID that identifies the peer type. It's supposed to be globally unique.
	peerImplementationClassUID string
	// Implementation version, virtually meaningless since its format isn't standardiszed.
	peerImplementationVersionName string

	// tmpRequests used only on the client (requestor) side. It holds the
	// contextid->presentationcontext mapping generated from the
	// A_ASSOCIATE_RQ PDU. Once an A_ASSOCIATE_AC PDU arrives, tmpRequests
	// is matched against the response PDU and
	// contextid->{abstractsyntax,transfersyntax} mappings are filled.
	tmpRequests map[byte]*pdu.PresentationContextItem
}

// Create an empty contextManager
func newContextManager(label string) *contextManager {
	c := &contextManager{
		label:                            label,
		contextIDToAbstractSyntaxNameMap: make(map[byte]*contextManagerEntry),
		abstractSyntaxNameToContextIDMap: make(map[string]*contextManagerEntry),
		peerMaxPDUSize:                   16384, // The default value used by Osirix & pynetdicom.
		tmpRequests:                      make(map[byte]*pdu.PresentationContextItem),
	}
	return c
}

// Called by the user (client) to produce a list to be embedded in an
// A_REQUEST_RQ.Items. The PDU is sent when running as a service user (client).
// maxPDUSize is the maximum PDU size, in bytes, that the clients is willing to
// receive. maxPDUSize is encoded in one of the items.
func (m *contextManager) generateAssociateRequest(
	sopClassUIDs []string, transferSyntaxUIDs []string) []pdu.SubItem {
	items := []pdu.SubItem{
		&pdu.ApplicationContextItem{
			Name: pdu.DICOMApplicationContextItemName,
		}}
	var contextID byte = 1
	for _, sop := range sopClassUIDs {
		syntaxItems := []pdu.SubItem{
			&pdu.AbstractSyntaxSubItem{Name: sop},
		}
		for _, syntaxUID := range transferSyntaxUIDs {
			syntaxItems = append(syntaxItems, &pdu.TransferSyntaxSubItem{Name: syntaxUID})
		}
		item := &pdu.PresentationContextItem{
			Type:      pdu.ItemTypePresentationContextRequest,
			ContextID: contextID,
			Result:    0, // must be zero for request
			Items:     syntaxItems,
		}
		items = append(items, item)
		m.tmpRequests[contextID] = item
		contextID += 2 // must be odd.
	}
	items = append(items,
		&pdu.UserInformationItem{
			Items: []pdu.SubItem{
				&pdu.UserInformationMaximumLengthItem{uint32(DefaultMaxPDUSize)},
				&pdu.ImplementationClassUIDSubItem{dicom.GoDICOMImplementationClassUID},
				&pdu.ImplementationVersionNameSubItem{dicom.GoDICOMImplementationVersionName}}})

	return items
}

// Called when A_ASSOCIATE_RQ pdu arrives, on the provider side. Returns a list of items to be sent in
// the A_ASSOCIATE_AC pdu.
func (m *contextManager) onAssociateRequest(requestItems []pdu.SubItem) ([]pdu.SubItem, error) {
	responses := []pdu.SubItem{
		&pdu.ApplicationContextItem{
			Name: pdu.DICOMApplicationContextItemName,
		},
	}
	for _, requestItem := range requestItems {
		switch ri := requestItem.(type) {
		case *pdu.PresentationContextItem:
			var sopUID string
			var pickedTransferSyntaxUID string
			for _, subItem := range ri.Items {
				switch c := subItem.(type) {
				case *pdu.AbstractSyntaxSubItem:
					if sopUID != "" {
						return nil, fmt.Errorf("dicom.onAssociateRequest: Multiple AbstractSyntaxSubItem found in %v",
							ri.String())
					}
					sopUID = c.Name
				case *pdu.TransferSyntaxSubItem:
					// Just pick the first syntax UID proposed by the client.
					if pickedTransferSyntaxUID == "" {
						pickedTransferSyntaxUID = c.Name
					}
				default:
					return nil, fmt.Errorf("dicom.onAssociateRequest: Unknown subitem in PresentationContext: %s",
						subItem.String())
				}
			}
			if sopUID == "" || pickedTransferSyntaxUID == "" {
				return nil, fmt.Errorf("dicom.onAssociateRequest: SOP or transfersyntax not found in PresentationContext: %v",
					ri.String())
			}
			responses = append(responses, &pdu.PresentationContextItem{
				Type:      pdu.ItemTypePresentationContextResponse,
				ContextID: ri.ContextID,
				Result:    0, // accepted
				Items:     []pdu.SubItem{&pdu.TransferSyntaxSubItem{Name: pickedTransferSyntaxUID}}})
			addContextMapping(m, sopUID, pickedTransferSyntaxUID, ri.ContextID, pdu.PresentationContextAccepted)
		case *pdu.UserInformationItem:
			for _, subItem := range ri.Items {
				switch c := subItem.(type) {
				case *pdu.UserInformationMaximumLengthItem:
					m.peerMaxPDUSize = int(c.MaximumLengthReceived)
				case *pdu.ImplementationClassUIDSubItem:
					m.peerImplementationClassUID = c.Name
				case *pdu.ImplementationVersionNameSubItem:
					m.peerImplementationVersionName = c.Name

				}
			}
		}
	}

	responses = append(responses,
		&pdu.UserInformationItem{
			Items: []pdu.SubItem{&pdu.UserInformationMaximumLengthItem{MaximumLengthReceived: uint32(DefaultMaxPDUSize)}}})

	logrus.WithFields(logrus.Fields{
		"Version": m.peerImplementationVersionName,
		"ID":      m.label,
	}).Info("Client")
	return responses, nil
}

// Called by the user (client) to when A_ASSOCIATE_AC PDU arrives from the provider.
func (m *contextManager) onAssociateResponse(responses []pdu.SubItem) error {
	for _, responseItem := range responses {
		switch ri := responseItem.(type) {
		case *pdu.PresentationContextItem:
			var pickedTransferSyntaxUID string
			for _, subItem := range ri.Items {
				switch c := subItem.(type) {
				case *pdu.TransferSyntaxSubItem:
					// Just pick the first syntax UID proposed by the client.
					if pickedTransferSyntaxUID == "" {
						pickedTransferSyntaxUID = c.Name
					} else {
						return fmt.Errorf("Multiple syntax UIDs returned in A_ASSOCIATE_AC: %v", ri.String())
					}
				default:
					return fmt.Errorf("Unknown subitem %s in PresentationContext: %s", subItem.String(), ri.String())
				}
			}
			request, ok := m.tmpRequests[ri.ContextID]
			if !ok {
				return fmt.Errorf("Unknown context ID %d for A_ASSOCIATE_AC: %v",
					ri.ContextID,
					ri.String())
			}
			var sopUID string
			for _, subItem := range request.Items {
				switch c := subItem.(type) {
				case *pdu.AbstractSyntaxSubItem:
					sopUID = c.Name
				case *pdu.TransferSyntaxSubItem:
					if c.Name == pickedTransferSyntaxUID {
						break
					}
				}
			}
			if sopUID == "" {
				return fmt.Errorf("dicom.onAssociateResponse(%s): The A-ASSOCIATE request lacks the abstract syntax item for tag %v (this shouldn't happen)", m.label, ri.ContextID)
			}
			addContextMapping(m, sopUID, pickedTransferSyntaxUID, ri.ContextID, ri.Result)
		case *pdu.UserInformationItem:
			for _, subItem := range ri.Items {
				switch c := subItem.(type) {
				case *pdu.UserInformationMaximumLengthItem:
					m.peerMaxPDUSize = int(c.MaximumLengthReceived)
				case *pdu.ImplementationClassUIDSubItem:
					m.peerImplementationClassUID = c.Name
				case *pdu.ImplementationVersionNameSubItem:
					m.peerImplementationVersionName = c.Name

				}
			}
		}
	}
	return nil
}

// Add a mapping between a (global) UID and a (per-session) context ID.
func addContextMapping(
	m *contextManager,
	abstractSyntaxUID string,
	transferSyntaxUID string,
	contextID byte,
	result pdu.PresentationContextResult) {

	doassert(result <= 4, result)
	doassert(contextID%2 == 1, contextID)
	if result == 0 {
		doassert(abstractSyntaxUID != "", abstractSyntaxUID)
		doassert(transferSyntaxUID != "", transferSyntaxUID)
	}
	e := &contextManagerEntry{
		abstractSyntaxUID: abstractSyntaxUID,
		transferSyntaxUID: transferSyntaxUID,
		contextID:         contextID,
		result:            result,
	}
	m.contextIDToAbstractSyntaxNameMap[contextID] = e
	m.abstractSyntaxNameToContextIDMap[abstractSyntaxUID] = e
}

func (m *contextManager) checkContextRejection(e *contextManagerEntry) error {
	if e.result != pdu.PresentationContextAccepted {
		return fmt.Errorf("dicom.checkContextRejection %v: Trying to use rejected context <%v, %v>: %s",
			m.label,
			dicomuid.UIDString(e.abstractSyntaxUID),
			dicomuid.UIDString(e.transferSyntaxUID),
			e.result.String())
	}
	return nil
}

// Convert an UID to a context ID.
func (m *contextManager) lookupByAbstractSyntaxUID(name string) (contextManagerEntry, error) {
	e, ok := m.abstractSyntaxNameToContextIDMap[name]
	if !ok {
		return contextManagerEntry{}, fmt.Errorf("dicom.checkContextRejection %v: Unknown syntax %s", m.label, dicomuid.UIDString(name))
	}
	err := m.checkContextRejection(e)
	if err != nil {
		return contextManagerEntry{}, err
	}
	return *e, nil
}

// Convert a contextID to a UID.
func (m *contextManager) lookupByContextID(contextID byte) (contextManagerEntry, error) {
	e, ok := m.contextIDToAbstractSyntaxNameMap[contextID]
	if !ok {
		return contextManagerEntry{}, fmt.Errorf("dicom.lookupByContextID %v: Unknown context ID %d", m.label, contextID)
	}
	err := m.checkContextRejection(e)
	if err != nil {
		return contextManagerEntry{}, err
	}
	return *e, nil
}
