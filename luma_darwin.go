// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package luma

import (
	"fmt"
	"time"

	"github.com/go-macos/iokit/usb"
)

// platform is the seam the portable model is tested against.
type platform interface {
	drain(times int, each time.Duration)
	write(b []byte, timeout time.Duration) error
	read(timeout time.Duration) ([]byte, error)
	close() error
}

// pipes is the real one: an open interface and its two pipe references.
//
// ⛔ THE REFS ARE NOT THE ADDRESSES. IOKit numbers an interface's pipes 1..n
// and takes THAT, while the descriptor and the vendor's code talk in endpoint
// addresses. On this headset ep 0x04 is pipe 4 and ep 0x85 is pipe 5, which
// looks like a coincidence worth not relying on -- so they are looked up.
type pipes struct {
	h        *usb.InterfaceHandle
	out, in  uint8
	inBuffer int
}

func open() (*Glasses, error) {
	ifs, err := usb.Interfaces(usb.InterfaceFilter{
		VendorID: VendorID, ProductIDs: []uint16{ProductID},
	})
	if err != nil {
		return nil, fmt.Errorf("luma: looking for the headset: %w", err)
	}
	if len(ifs) == 0 {
		return nil, ErrNoDevice
	}
	h := ifs[0]
	if err := h.Open(); err != nil {
		return nil, fmt.Errorf("luma: opening the headset: %w", err)
	}
	ps, err := h.Pipes()
	if err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("luma: reading its pipes: %w", err)
	}
	p := &pipes{h: h, inBuffer: 64}
	for _, one := range ps {
		switch one.Address() {
		case EndpointOut:
			p.out = one.Ref
		case EndpointIn:
			p.in = one.Ref
			if int(one.MaxPacket) > p.inBuffer {
				p.inBuffer = int(one.MaxPacket)
			}
		}
	}
	if p.out == 0 || p.in == 0 {
		_ = h.Close()
		return nil, fmt.Errorf("%w: it has no endpoint %#02x and %#02x",
			ErrNoDevice, EndpointOut, EndpointIn)
	}
	return &Glasses{p: p}, nil
}

func (p *pipes) drain(times int, each time.Duration) {
	buf := make([]byte, p.inBuffer)
	for i := 0; i < times; i++ {
		n, err := p.h.Read(p.in, buf, each)
		if err != nil || n == 0 {
			return
		}
	}
}

func (p *pipes) write(b []byte, timeout time.Duration) error {
	if _, err := p.h.Write(p.out, b, timeout); err != nil {
		return fmt.Errorf("luma: asking the headset: %w", err)
	}
	return nil
}

func (p *pipes) read(timeout time.Duration) ([]byte, error) {
	buf := make([]byte, p.inBuffer)
	n, err := p.h.Read(p.in, buf, timeout)
	if err != nil {
		return nil, fmt.Errorf("luma: waiting for its answer: %w", err)
	}
	return buf[:n], nil
}

func (p *pipes) close() error { return p.h.Close() }
