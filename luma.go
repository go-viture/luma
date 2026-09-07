// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package luma

import (
	"errors"
	"time"
)

// ErrUnsupported is returned off macOS.
var ErrUnsupported = errors.New("luma: only macOS is wired up")

// ErrNoDevice says no Luma Ultra is on the bus.
var ErrNoDevice = errors.New("luma: no VITURE Luma Ultra found")

// How long each half of an exchange is given.
//
// ⭐ THE VENDOR'S OWN NUMBERS: 100 ms for the drain and 1500 ms for the command
// and its answer, read out of the getters this package was decoded from.
const (
	// DrainTimeout bounds each read of the leftover-clearing pass.
	DrainTimeout = 100 * time.Millisecond
	// Timeout bounds the command and its answer.
	Timeout = 1500 * time.Millisecond
	// DrainReads is how many leftover frames are cleared before asking.
	DrainReads = 4
)

// Glasses is one open headset.
type Glasses struct {
	p platform
}

// Open finds the headset and claims its vendor interface.
//
// ⛔ THE INTERFACE IS EXCLUSIVE. VITURE's own SpaceWalker holds it while it
// runs, and Open then fails with the system's exclusive-access error rather
// than silently reading nothing. Measured: it opens freely with that app
// closed and is refused with it running.
func Open() (*Glasses, error) { return openDevice() }

// openDevice is the platform call, replaced in tests so that everything above
// the transport is exercised on every operating system.
var openDevice = open

// Close releases it.
func (g *Glasses) Close() error {
	if g == nil {
		return nil
	}
	return g.p.close()
}

// Ask sends one command and returns what came back.
//
// ⛔ IT DRAINS FIRST, and that is not housekeeping. The vendor's code clears
// the inbound pipe before EVERY command, because the pipe is shared and a late
// answer to an earlier question is indistinguishable from an early answer to
// this one -- it has the right shape, it arrives at the right moment, and it
// says the wrong thing. The echoed command byte catches what the drain misses.
func (g *Glasses) Ask(command byte, payload []byte) (Reply, error) {
	if g == nil {
		return Reply{}, ErrNoDevice
	}
	g.p.drain(DrainReads, DrainTimeout)
	if err := g.p.write(Request(command, payload), Timeout); err != nil {
		return Reply{}, err
	}
	b, err := g.p.read(Timeout)
	if err != nil {
		return Reply{}, err
	}
	return ParseReply(b, command)
}

// Commands this package has PROVEN on the hardware.
//
// ⛔ NOTHING HERE IS GUESSED. Both are frames the vendor's own
// carina_a1088_get_firmware_version sends, which is why they were safe to try:
// a version read changes nothing. Twenty-five other command bytes are known to
// exist and are deliberately absent, because knowing a number is not knowing
// what it does -- and an unknown opcode written to a headset somebody is
// wearing is not a probe.
const (
	// CmdVersionA answered "02 01 09" on the headset this was written for.
	CmdVersionA byte = 0x80
	// CmdVersionB answered "00", and is the second frame the vendor's getter
	// sends. What distinguishes the two is not established.
	CmdVersionB byte = 0xc3
)

// ChipVersion is the version of the CHIP's firmware, and not the product's.
//
// ⛔⛔ IT WAS CALLED Version, AND THAT WAS WRONG. It answers "02 01 09" on a
// headset whose own updater reads 0.01.101_20260605, and an earlier note took
// those three bytes for "firmware 2.1.9". The vendor's library settles it:
// carina_a1088_get_firmware_version SENDS NOTHING -- it hands back a cached
// string filled by a routine that picks between two getters on a model field,
// and the getter this headset reaches NAMES ITSELF in its own log strings,
// "getchipfwver" and "get chip fw ver data status". So 0x80 is a correct read
// of the chip's firmware; only the label was false.
//
// ⚠ AND THE BYTES ARE STILL JUST BYTES. Nothing confirms that "02 01 09" is to
// be shown as 2.1.9. A caller that wants to print it can; this will not pretend
// on its behalf.
//
// For the version a person recognises, and the serial the vendor prints on its
// own screen, see [Glasses.Info].
func (g *Glasses) ChipVersion() ([]byte, error) {
	r, err := g.Ask(CmdVersionA, nil)
	if err != nil {
		return nil, err
	}
	return r.Rest, nil
}

// platform is the seam the portable model is tested against.
//
// ⛔ IT LIVES HERE, ONCE. It used to be declared separately in each transport
// file, and adding a method to one of them built on that platform and broke
// every other -- six red checks for a seam that only ever had one meaning.
type platform interface {
	drain(times int, each time.Duration)
	write(b []byte, timeout time.Duration) error
	read(timeout time.Duration) ([]byte, error)
	// exchange sends one packet on the DATA endpoints and reads the answer.
	// Separate from write/read because it is a different pair of pipes: the
	// MCU pair is reserved for flashing.
	exchange(b []byte, timeout time.Duration) ([]byte, error)
	close() error
}
