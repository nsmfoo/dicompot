// Code generated by "stringer -type QRLevel"; DO NOT EDIT

package dicompot

import "fmt"

const _QRLevel_name = "QRLevelPatientQRLevelStudyQRLevelSeries"

var _QRLevel_index = [...]uint8{0, 14, 26, 39}

func (i QRLevel) String() string {
	if i < 0 || i >= QRLevel(len(_QRLevel_index)-1) {
		return fmt.Sprintf("QRLevel(%d)", i)
	}
	return _QRLevel_name[_QRLevel_index[i]:_QRLevel_index[i+1]]
}
