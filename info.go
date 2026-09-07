// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package luma

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// ⭐⭐ THIS HEADSET SPEAKS TWO ENVELOPES, AND WE ONLY KNEW ONE.
//
// The "FA 55 | cmd | len16BE" frames the rest of this package sends on
// endpoints 0x04/0x85 are the MCU channel. VITURE's own web updater uses it for
// exactly three things -- MT_FW_START, MT_FW_WRITE, MT_FW_READ -- which is to
// say FLASHING, and its native SDK adds a chip-version read (see
// [Glasses.ChipVersion]).
//
// Everything a person would call "the firmware version" travels on a DIFFERENT
// envelope, over the data endpoints, in 512-byte packets:
//
//	0..1    sync: FF FE host->device, FF FD device->host
//	2..3    CRC-16/XMODEM over bytes[4 : 6+pklen], LITTLE-endian
//	4..5    pklen, little-endian: 12 + len(payload)
//	6..13   eight bytes the device fills with a counter; zero on a request
//	14..15  message id, little-endian
//	16..17  zero on a request
//	18      status on a reply: 0 is success
//	19..    payload, ASCII, padded with NULs in the fixed-width fields
//
// ⛔ AND THE ENDPOINTS ARE NOT 0x04/0x85. Measured on a Luma Ultra: 0x06 out
// and 0x87 in answer; 0x04/0x85 are the MCU pair and must be left to flashing.
//
// ⚠ THE VENDOR'S TOOL TRIMS WHAT IT SHOWS. The headset answers
// "12.0.01.101_20260605" and the updater displays "0.01.101_20260605". This
// package hands over what the DEVICE said and lets a caller decide -- a library
// that silently dropped a prefix would be a library nobody could check against
// the vendor's own screen.

// Message ids on the data envelope. Every one of these is a READ.
const (
	MsgAppFirmwareVersion uint16 = 0x0001
	MsgDPVersion          uint16 = 0x000b
	MsgBoardSerial        uint16 = 0x0010
	MsgPackageSerial      uint16 = 0x0011
	MsgOSDVersion         uint16 = 0x0063
)

// Data endpoints, as measured. See the comment above for why they are not the
// MCU pair.
const (
	DataEndpointOut byte = 0x06
	DataEndpointIn  byte = 0x87
)

// dataPacket is the fixed size of both directions on this envelope.
const dataPacket = 512

// dataHeader is where a reply's payload starts.
const dataHeader = 19

// ErrRefused says the headset answered with a non-zero status.
var ErrRefused = errors.New("luma: the headset refused the request")

// Info is what the headset says about itself.
type Info struct {
	// FirmwareVersion is the application firmware, VERBATIM. The vendor's
	// updater shows it without its leading component: "12.0.01.101_20260605"
	// here is "0.01.101_20260605" there.
	FirmwareVersion string
	// BoardSerial identifies the board; PackageSerial identifies the product,
	// and is the one the vendor's updater labels "SN".
	BoardSerial, PackageSerial string
}

// crc16XModem is the check the device applies to this envelope: poly 0x1021,
// init zero, no reflection, no final xor.
func crc16XModem(b []byte) uint16 {
	var crc uint16
	for _, c := range b {
		crc ^= uint16(c) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// DataRequest builds one request on the data envelope, padded to the packet
// size the device expects.
func DataRequest(msg uint16, payload []byte) []byte {
	pkt := make([]byte, dataPacket)
	pkt[0], pkt[1] = 0xff, 0xfe
	pklen := 12 + len(payload)
	binary.LittleEndian.PutUint16(pkt[4:], uint16(pklen))
	binary.LittleEndian.PutUint16(pkt[14:], msg)
	copy(pkt[18:], payload)
	// The check covers from the length field to the end of the payload, which
	// is 6+pklen -- NOT the whole packet, and not the padding after it.
	binary.LittleEndian.PutUint16(pkt[2:], crc16XModem(pkt[4:6+pklen]))
	return pkt
}

// ParseData reads a reply on the data envelope.
//
// ⛔ IT TRIMS TRAILING NULs AND NOTHING ELSE. The serial fields are fixed-width
// and come NUL-padded, so a caller that kept them would put invisible bytes in
// a settings window; a caller that trimmed spaces too would be guessing.
func ParseData(b []byte, want uint16) (string, error) {
	if len(b) < dataHeader {
		return "", fmt.Errorf("luma: the answer is %d bytes, too short to hold a header", len(b))
	}
	if b[0] != 0xff || b[1] != 0xfd {
		return "", fmt.Errorf("luma: the answer does not start FF FD but %#02x %#02x", b[0], b[1])
	}
	if got := binary.LittleEndian.Uint16(b[14:]); got != want {
		return "", fmt.Errorf("luma: asked message %#04x and %#04x answered", want, got)
	}
	if st := b[18]; st != 0 {
		return "", fmt.Errorf("%w: status %d", ErrRefused, st)
	}
	end := 6 + int(binary.LittleEndian.Uint16(b[4:]))
	if end > len(b) {
		end = len(b)
	}
	if end <= dataHeader {
		return "", nil
	}
	return strings.TrimRight(string(b[dataHeader:end]), "\x00"), nil
}

// AskData sends one data-envelope request and returns the text it answered.
func (g *Glasses) AskData(msg uint16) (string, error) {
	if g == nil {
		return "", ErrNoDevice
	}
	b, err := g.p.exchange(DataRequest(msg, nil), Timeout)
	if err != nil {
		return "", err
	}
	return ParseData(b, msg)
}

// Info asks the headset what it is: the firmware a person would recognise, and
// its two serial numbers.
//
// ⛔ IT DOES NOT FALL BACK. A version this could not read is left empty rather
// than filled with the chip version, which is a different number entirely and
// would be wrong on screen in a way nobody could catch.
func (g *Glasses) Info() (Info, error) {
	var in Info
	var first error
	keep := func(dst *string, msg uint16) {
		s, err := g.AskData(msg)
		if err != nil {
			if first == nil {
				first = err
			}
			return
		}
		*dst = s
	}
	keep(&in.FirmwareVersion, MsgAppFirmwareVersion)
	keep(&in.BoardSerial, MsgBoardSerial)
	keep(&in.PackageSerial, MsgPackageSerial)
	if in.FirmwareVersion == "" && in.BoardSerial == "" && in.PackageSerial == "" {
		return in, first
	}
	return in, nil
}
