// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package luma

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// ⛔⛔ WHY THESE TESTS LOOK THE WAY THEY DO.
//
// The test this replaces built a fake that replayed the bytes I had observed
// and then asserted the package returned them. It could not fail, and it froze
// a misreading: three bytes called "the firmware version" on a headset whose
// own updater says 0.01.101_20260605.
//
// So the requests below are NOT computed by this package and compared with
// themselves. They are the frames VITURE's own WebAssembly module emits,
// captured from it, and the replies are what THIS HEADSET actually sent back.
// If the CRC here were wrong, or the layout misread, these would fail -- which
// is the whole point of a judge that is not us.

// The vendor's own request frames, first 16 bytes (the rest is zero padding).
var vendorRequests = map[uint16]string{
	MsgAppFirmwareVersion: "fffe7a600c00000000000000000001 00",
	MsgBoardSerial:        "fffe690d0c00000000000000000010 00",
	MsgPackageSerial:      "fffedd7b0c00000000000000000011 00",
	MsgDPVersion:          "fffed1080c0000000000000000000b 00",
	MsgOSDVersion:         "fffec0d40c00000000000000000063 00",
}

func TestDataRequestMatchesTheVendorsOwnBytes(t *testing.T) {
	for msg, want := range vendorRequests {
		wantB, err := hex.DecodeString(strings.ReplaceAll(want, " ", ""))
		if err != nil {
			t.Fatalf("%#04x: bad fixture: %v", msg, err)
		}
		got := DataRequest(msg, nil)
		if len(got) != dataPacket {
			t.Errorf("%#04x: packet is %d bytes, want %d", msg, len(got), dataPacket)
		}
		if h := hex.EncodeToString(got[:len(wantB)]); h != hex.EncodeToString(wantB) {
			t.Errorf("%#04x:\n got %s\nwant %s\n(the CRC or the layout is wrong; these bytes come "+
				"from the vendor's own module, not from us)", msg, h, hex.EncodeToString(wantB))
		}
		for i, c := range got[len(wantB):] {
			if c != 0 {
				t.Errorf("%#04x: byte %d of the padding is %#02x, want zero", msg, len(wantB)+i, c)
				break
			}
		}
	}
}

// The replies this headset actually sent, captured 2026-09-07.
const (
	replyFirmware = "fffdaf0f210000000000763e42000100000000" + "31322e302e30312e3130315f3230323630363035"
	replyBoardSN  = "fffd097a210000000000773e42001000000000" + "503653504e483534373031313332000000000000"
	replyPkgSN    = "fffd74a7210000000000783e42001100000000" + "533135343830313130310000000000000000000000"
)

func TestParseDataOnWhatTheHeadsetActuallySent(t *testing.T) {
	for _, c := range []struct {
		what, raw string
		msg       uint16
		want      string
	}{
		// ⚠ The vendor's updater shows this WITHOUT its leading "12.". The
		// headset says the whole thing, and so does this package: a library
		// that trimmed it could never be checked against the vendor's screen.
		{"firmware", replyFirmware, MsgAppFirmwareVersion, "12.0.01.101_20260605"},
		{"board serial", replyBoardSN, MsgBoardSerial, "P6SPNH54701132"},
		{"package serial", replyPkgSN, MsgPackageSerial, "S154801101"},
	} {
		b, err := hex.DecodeString(c.raw)
		if err != nil {
			t.Fatalf("%s: bad fixture: %v", c.what, err)
		}
		got, err := ParseData(b, c.msg)
		if err != nil {
			t.Fatalf("%s: ParseData: %v", c.what, err)
		}
		if got != c.want {
			t.Errorf("%s = %q, want %q", c.what, got, c.want)
		}
		if strings.ContainsRune(got, 0) {
			t.Errorf("%s = %q: the fixed-width fields are NUL-padded and the padding "+
				"must not reach a settings window", c.what, got)
		}
	}
}

func TestParseDataRefusesWhatItCannotTrust(t *testing.T) {
	good, _ := hex.DecodeString(replyFirmware)

	t.Run("a reply to another question", func(t *testing.T) {
		if _, err := ParseData(good, MsgBoardSerial); err == nil {
			t.Error("a firmware reply was accepted as a serial: replies must be matched by id")
		}
	})
	t.Run("a non-zero status", func(t *testing.T) {
		bad := append([]byte(nil), good...)
		bad[18] = 3
		_, err := ParseData(bad, MsgAppFirmwareVersion)
		if !errors.Is(err, ErrRefused) {
			t.Errorf("status 3 gave %v, want an ErrRefused", err)
		}
	})
	t.Run("the wrong sync word", func(t *testing.T) {
		bad := append([]byte(nil), good...)
		bad[1] = 0xfe // FF FE is the HOST's word; a device never sends it
		if _, err := ParseData(bad, MsgAppFirmwareVersion); err == nil {
			t.Error("a host-shaped frame was accepted as a reply")
		}
	})
	t.Run("truncated", func(t *testing.T) {
		if _, err := ParseData(good[:10], MsgAppFirmwareVersion); err == nil {
			t.Error("ten bytes were accepted as a reply")
		}
	})
}

// ⛔ A version that could not be read must stay EMPTY. Filling it with the chip
// version -- the mistake this whole file exists because of -- would put a wrong
// number on screen that nobody could catch.
func TestInfoNeverSubstitutesTheChipVersion(t *testing.T) {
	fw, _ := hex.DecodeString(replyFirmware)
	chip := []byte{0xfa, 0x55, 0x80, 0, 0, 2, 1, 9}

	t.Run("everything refused leaves everything empty", func(t *testing.T) {
		g := &Glasses{p: &fake{answers: [][]byte{chip, chip, chip}}}
		in, err := g.Info()
		if err == nil {
			t.Error("three unparseable answers reported success")
		}
		if in.FirmwareVersion != "" || in.BoardSerial != "" || in.PackageSerial != "" {
			t.Errorf("Info = %+v, want everything empty", in)
		}
	})

	t.Run("one good answer is kept and the rest stay empty", func(t *testing.T) {
		g := &Glasses{p: &fake{answers: [][]byte{fw, chip, chip}}}
		in, _ := g.Info()
		if in.FirmwareVersion != "12.0.01.101_20260605" {
			t.Errorf("FirmwareVersion = %q", in.FirmwareVersion)
		}
		if in.BoardSerial != "" || in.PackageSerial != "" {
			t.Errorf("a serial was invented: %+v", in)
		}
	})
}
