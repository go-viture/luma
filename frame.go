// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package luma talks to a VITURE Luma Ultra headset in pure Go, with
// CGO_ENABLED=0.
//
// ⛔ THE HEADSET NEVER SPEAKS FIRST. Measured 2026-09-06 over 75 seconds, with
// the buttons on the arm pressed and the head moving, listening on all five of
// its inbound pipes: twelve identical frames in the first ten milliseconds --
// a buffer flushed at open -- and then nothing at all. It is a request and
// response device, so no amount of listening decodes it. Every fact here came
// from asking.
//
// That is the opposite of the Beast, which ANNOUNCES what its own buttons do
// and can be decoded by watching. Do not carry habits from one to the other:
// they do not even share an envelope.
package luma

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// The device, and the two pipes that carry the protocol.
//
// ⭐ MEASURED, not assumed. 35ca:1104 publishes seven endpoints; the two below
// are the ones the vendor's own getters use, and the ones this package proved
// on the hardware.
const (
	// VendorID and ProductID are the Luma Ultra's raw vendor interface. The
	// companion 35ca:1102 is NOT this: it is an audio-key set whose name --
	// "VITURE Microphone" -- has now misled two separate investigations.
	VendorID  uint16 = 0x35ca
	ProductID uint16 = 0x1104

	// EndpointOut and EndpointIn are where a command goes and where its answer
	// comes back.
	EndpointOut byte = 0x04
	EndpointIn  byte = 0x85
)

// Magic is the two bytes every frame begins with, in the order they go on the
// wire.
var Magic = [2]byte{0xFA, 0x55}

// ErrNotAFrame says a reply did not begin with [Magic].
var ErrNotAFrame = errors.New("luma: not a frame")

// ErrWrongCommand says a reply answered a different command than the one asked.
//
// ⭐ THE HEADSET REPEATS THE COMMAND BYTE, which is what makes this checkable
// without a sequence number -- and worth checking, because the pipe is shared
// and a late answer to an earlier question looks exactly like an early answer
// to this one.
var ErrWrongCommand = errors.New("luma: the reply answers a different command")

// Request builds the frame that asks for a command.
//
//	FA 55 | command | length | payload
//
// ⚠ THE LENGTH IS BIG-ENDIAN HERE, AND ONLY HERE. The vendor's frame builders
// store it with a 16-bit little-endian write of 0x0100, 0x0200, 0x0300, 0x0800
// and 0x4000 -- which put the bytes 00 01, 00 02, 00 03, 00 08, 00 40 on the
// wire, and the number of payload bytes that follow matches every time when
// they are read big-endian. A REPLY appears to use the other order, and that
// is not yet settled: see [Reply].
func Request(command byte, payload []byte) []byte {
	b := make([]byte, 5+len(payload))
	b[0], b[1] = Magic[0], Magic[1]
	b[2] = command
	binary.BigEndian.PutUint16(b[3:5], uint16(len(payload)))
	copy(b[5:], payload)
	return b
}

// Reply is what came back, split as far as it is understood.
//
// ⚠ THE LENGTH FIELD OF A REPLY IS NOT SETTLED, so this hands back the two
// bytes AND everything after them rather than pretending to know. Measured:
//
//	fa 55 80 00 00 02 01 09     "00 00" then THREE bytes
//	fa 55 c3 01 00 00           "01 00" then ONE byte
//
// Read little-endian the second is right and the first is not. Two readings
// remain open -- those bytes may not be a length at all in a reply -- and
// guessing here is how nineteen attempts were lost on the sibling headset,
// which turned out to answer in one byte order and accept commands in another.
type Reply struct {
	// Command is the byte the headset echoed back.
	Command byte
	// Field is bytes 3 and 4, whatever they mean.
	Field [2]byte
	// Rest is everything after them, unparsed.
	Rest []byte
}

// String renders a reply the way a probe prints it.
func (r Reply) String() string {
	return fmt.Sprintf("command %#02x field % x rest % x", r.Command, r.Field, r.Rest)
}

// ParseReply splits a frame the headset sent.
func ParseReply(b []byte, want byte) (Reply, error) {
	if len(b) < 5 || b[0] != Magic[0] || b[1] != Magic[1] {
		return Reply{}, fmt.Errorf("%w: % x", ErrNotAFrame, b)
	}
	r := Reply{Command: b[2], Field: [2]byte{b[3], b[4]}, Rest: b[5:]}
	if b[2] != want {
		return r, fmt.Errorf("%w: asked %#02x, got %#02x", ErrWrongCommand, want, b[2])
	}
	return r, nil
}
