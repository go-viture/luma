// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package luma

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// fake stands in for the headset: it records what was asked and answers from a
// script.
type fake struct {
	drains  int
	drained int
	wrote   [][]byte
	answers [][]byte
	readErr error
	wrErr   error
}

func (f *fake) drain(times int, _ time.Duration) { f.drains++; f.drained = times }
func (f *fake) write(b []byte, _ time.Duration) error {
	if f.wrErr != nil {
		return f.wrErr
	}
	f.wrote = append(f.wrote, append([]byte(nil), b...))
	return nil
}
func (f *fake) read(time.Duration) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	if len(f.answers) == 0 {
		return nil, errors.New("nothing left to say")
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return a, nil
}
func (f *fake) close() error { return nil }

// TestAskDrainsBeforeEveryCommand.
//
// ⛔ THE DRAIN IS NOT HOUSEKEEPING. The inbound pipe is shared, so a late
// answer to an earlier question is indistinguishable from an early answer to
// this one: it has the right shape and arrives at the right moment. The
// vendor's own code clears the pipe before EVERY command, and this does too --
// once per Ask, not once per session.
func TestAskDrainsBeforeEveryCommand(t *testing.T) {
	f := &fake{answers: [][]byte{
		{0xfa, 0x55, 0x80, 0, 0, 2, 1, 9},
		{0xfa, 0x55, 0xc3, 1, 0, 0},
	}}
	g := &Glasses{p: f}

	if _, err := g.Ask(CmdVersionA, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Ask(CmdVersionB, nil); err != nil {
		t.Fatal(err)
	}
	if f.drains != 2 {
		t.Errorf("two commands drained the pipe %d times, want 2", f.drains)
	}
	if f.drained != DrainReads {
		t.Errorf("each drain cleared %d frames, want %d", f.drained, DrainReads)
	}
	if len(f.wrote) != 2 {
		t.Fatalf("it wrote %d frames", len(f.wrote))
	}
	if f.wrote[0][2] != CmdVersionA || f.wrote[1][2] != CmdVersionB {
		t.Errorf("it asked %#02x then %#02x", f.wrote[0][2], f.wrote[1][2])
	}
}

// TestVersionHandsBackTheBytesAndNotAStory.
//
// ⚠ "02 01 09" READS AS 2.1.9 AND THAT IS A GUESS about presentation, not a
// measurement: nothing has confirmed the field order, or that all three are
// version numbers. The package hands back the bytes.
func TestChipVersionIsTheChipsAndSaysSo(t *testing.T) {
	f := &fake{answers: [][]byte{{0xfa, 0x55, 0x80, 0, 0, 2, 1, 9}}}
	g := &Glasses{p: f}
	got, err := g.ChipVersion()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 2 || got[1] != 1 || got[2] != 9 {
		t.Errorf("Version() = % x", got)
	}
}

// TestEveryWayAnExchangeCanFail, and each says which.
func TestEveryWayAnExchangeCanFail(t *testing.T) {
	var none *Glasses
	if _, err := none.Ask(CmdVersionA, nil); !errors.Is(err, ErrNoDevice) {
		t.Errorf("asking nothing = %v", err)
	}
	if err := none.Close(); err != nil {
		t.Errorf("closing nothing = %v", err)
	}
	if _, err := none.ChipVersion(); !errors.Is(err, ErrNoDevice) {
		t.Errorf("no device version = %v", err)
	}

	boom := errors.New("the cable")
	g := &Glasses{p: &fake{wrErr: boom}}
	if _, err := g.Ask(CmdVersionA, nil); !errors.Is(err, boom) {
		t.Errorf("a refused write = %v", err)
	}
	g = &Glasses{p: &fake{readErr: boom}}
	if _, err := g.Ask(CmdVersionA, nil); !errors.Is(err, boom) {
		t.Errorf("a silent headset = %v", err)
	}
	// An answer to a different question is reported, not returned as if it fit.
	g = &Glasses{p: &fake{answers: [][]byte{{0xfa, 0x55, 0xc3, 0, 0}}}}
	if _, err := g.Ask(CmdVersionA, nil); !errors.Is(err, ErrWrongCommand) {
		t.Errorf("a stale answer = %v", err)
	}
	if err := (&Glasses{p: &fake{}}).Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}

// TestOpenReportsWhatThePlatformSaid, on every operating system.
//
// ⭐ THE SEAM IS WHAT MAKES THIS PORTABLE. Everything above the transport --
// the frames, the drain, the echoed-command check -- is the same code on every
// platform, and a package whose logic could only be tested where the hardware
// is would be a package tested on one machine.
func TestOpenReportsWhatThePlatformSaid(t *testing.T) {
	was := openDevice
	t.Cleanup(func() { openDevice = was })

	openDevice = func() (*Glasses, error) { return nil, ErrNoDevice }
	if _, err := Open(); !errors.Is(err, ErrNoDevice) {
		t.Errorf("Open with nothing attached = %v", err)
	}

	want := &Glasses{p: &fake{}}
	openDevice = func() (*Glasses, error) { return want, nil }
	got, err := Open()
	if err != nil || got != want {
		t.Errorf("Open() = %v, %v", got, err)
	}
}

// exchange is the data envelope's seam. The MCU pair and the data pair are
// different pipes, so a fake that conflated them would let a test pass while
// the real code sent a data frame down the flashing channel.
func (f *fake) exchange(b []byte, _ time.Duration) ([]byte, error) {
	f.wrote = append(f.wrote, append([]byte(nil), b...))
	if f.wrErr != nil {
		return nil, f.wrErr
	}
	if f.readErr != nil {
		return nil, f.readErr
	}
	if len(f.answers) == 0 {
		return nil, errTestExhausted
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return a, nil
}

var errTestExhausted = fmt.Errorf("fake: nothing left to answer")
