// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package luma

import (
	"bytes"
	"errors"
	"testing"
)

// TestTheFramesTheHeadsetActuallyAnswered.
//
// ⭐ THESE ARE MEASUREMENTS, not examples. Both were sent to a Luma Ultra on
// 2026-09-06 and both were answered, which is the only reason they are here:
//
//	fa 55 80 00 00  ->  fa 55 80 00 00 02 01 09
//	fa 55 c3 00 00  ->  fa 55 c3 01 00 00
//
// A test built from a guessed frame would pass just as well and mean nothing.
func TestTheFramesTheHeadsetActuallyAnswered(t *testing.T) {
	if got, want := Request(CmdVersionA, nil), []byte{0xfa, 0x55, 0x80, 0x00, 0x00}; !bytes.Equal(got, want) {
		t.Errorf("Request(%#02x) = % x, want % x", CmdVersionA, got, want)
	}
	if got, want := Request(CmdVersionB, nil), []byte{0xfa, 0x55, 0xc3, 0x00, 0x00}; !bytes.Equal(got, want) {
		t.Errorf("Request(%#02x) = % x, want % x", CmdVersionB, got, want)
	}

	r, err := ParseReply([]byte{0xfa, 0x55, 0x80, 0x00, 0x00, 0x02, 0x01, 0x09}, CmdVersionA)
	if err != nil {
		t.Fatalf("the answer the headset gave does not parse: %v", err)
	}
	if r.Command != CmdVersionA {
		t.Errorf("command %#02x", r.Command)
	}
	if !bytes.Equal(r.Rest, []byte{0x02, 0x01, 0x09}) {
		t.Errorf("rest % x", r.Rest)
	}
	// ⚠ AND THE FIELD IS HANDED BACK RAW. "00 00" here with three bytes after
	// it, "01 00" with one after it in the other reply: read as a
	// little-endian length the second works and the first does not. Until that
	// is settled this package must not pretend either way.
	if r.Field != [2]byte{0x00, 0x00} {
		t.Errorf("field % x", r.Field)
	}
	r2, err := ParseReply([]byte{0xfa, 0x55, 0xc3, 0x01, 0x00, 0x00}, CmdVersionB)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Field != [2]byte{0x01, 0x00} || !bytes.Equal(r2.Rest, []byte{0x00}) {
		t.Errorf("the second answer parsed as %v", r2)
	}
}

// TestTheLengthGoesOnBigEndian.
//
// ⚠ THE ORDER IS THE WHOLE POINT, and it is the trap that cost nineteen
// attempts on the sibling headset -- which answers in one byte order and
// accepts commands in another. The vendor's builders write 0x0100, 0x0200,
// 0x0300, 0x0800 and 0x4000 with a 16-bit little-endian store, putting 00 01,
// 00 02, 00 03, 00 08, 00 40 on the wire; the payload byte count that follows
// matches every one of those read BIG-endian.
func TestTheLengthGoesOnBigEndian(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 8, 64} {
		b := Request(0x06, make([]byte, n))
		if len(b) != 5+n {
			t.Fatalf("a %d-byte payload made a %d-byte frame", n, len(b))
		}
		if got := int(b[3])<<8 | int(b[4]); got != n {
			t.Errorf("%d payload bytes wrote length % x", n, b[3:5])
		}
	}
	// The one the vendor's switch_display_mode sends, byte for byte.
	if got, want := Request(0x06, []byte{0x02}),
		[]byte{0xfa, 0x55, 0x06, 0x00, 0x01, 0x02}; !bytes.Equal(got, want) {
		t.Errorf("the display-mode frame is % x, want % x", got, want)
	}
}

// TestARefusalSaysWhichKind: a caller has to tell "that is not one of ours"
// from "that is an answer to a different question", because the second is a
// pipe that needs draining and the first is a device that is not this one.
func TestARefusalSaysWhichKind(t *testing.T) {
	if _, err := ParseReply([]byte{0x10, 0x00, 0x24}, 0x24); !errors.Is(err, ErrNotAFrame) {
		t.Errorf("a Beast frame parsed as ours: %v", err)
	}
	if _, err := ParseReply(nil, 0x80); !errors.Is(err, ErrNotAFrame) {
		t.Errorf("nothing parsed as %v", err)
	}
	if _, err := ParseReply([]byte{0xfa, 0x55, 0x80, 0, 0}, 0x80); err != nil {
		t.Errorf("a five-byte answer refused: %v", err)
	}
	// ⭐ THE ECHOED COMMAND IS WHAT MAKES THIS CHECKABLE without a sequence
	// number, and it is worth checking: a late answer to an earlier question
	// has the right shape and arrives at the right moment.
	r, err := ParseReply([]byte{0xfa, 0x55, 0xc3, 0, 0}, 0x80)
	if !errors.Is(err, ErrWrongCommand) {
		t.Errorf("an answer to another question passed: %v", err)
	}
	if r.Command != 0xc3 {
		t.Errorf("the refusal threw away what it did answer: %v", r)
	}
	if got := r.String(); got == "" {
		t.Error("a reply renders as nothing")
	}
}
