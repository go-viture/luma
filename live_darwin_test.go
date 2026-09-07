// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package luma

import (
	"errors"
	"os"
	"testing"
)

// TestLiveVersion asks a real headset for its firmware version.
//
// ⛔ GATED, BECAUSE IT NEEDS HARDWARE AND EXCLUSIVITY. Set LUMA_LIVE=1 with a
// Luma Ultra plugged in and VITURE's SpaceWalker CLOSED -- that application
// holds the vendor interface while it runs, and this then fails with the
// system's exclusive-access error rather than reading nothing.
//
// ⭐ IT ASKS FOR A VERSION AND NOTHING ELSE. A version read changes no state,
// repeats safely, and its success is visible -- which is the whole reason it
// was the first thing ever sent to this headset. Twenty-five other command
// bytes are known to exist; none is exercised here, because knowing a number
// is not knowing what it does.
func TestLiveVersion(t *testing.T) {
	if os.Getenv("LUMA_LIVE") == "" {
		t.Skip("set LUMA_LIVE=1 with a Luma Ultra attached and SpaceWalker closed")
	}
	g, err := Open()
	if err != nil {
		if errors.Is(err, ErrNoDevice) {
			t.Skip("no Luma Ultra attached")
		}
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = g.Close() }()

	v, err := g.ChipVersion()
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if len(v) == 0 {
		t.Fatal("the headset answered with no payload at all")
	}
	t.Logf("firmware bytes: % x", v)

	// And the second frame the vendor's getter sends, which answers something
	// different -- proving the transport carries more than one command.
	r, err := g.Ask(CmdVersionB, nil)
	if err != nil {
		t.Fatalf("Ask(%#02x): %v", CmdVersionB, err)
	}
	t.Logf("%#02x answered: %v", CmdVersionB, r)
}
