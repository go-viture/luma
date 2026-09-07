// Copyright (c) the go-viture authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build !darwin

package luma

// open reports [ErrUnsupported].
//
// The frame model in this package is portable and tested everywhere; only the
// transport is macOS for now. Saying so beats handing back a headset that
// answers nothing, which reads like a headset that is not plugged in.
func open() (*Glasses, error) { return nil, ErrUnsupported }
