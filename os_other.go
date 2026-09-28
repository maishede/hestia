//go:build !windows

package main

import "errors"

func hideConsole(bool) {}

func toggleConsole() {}

func setAutoStart(bool) error { return errors.New("开机自启仅支持 Windows") }
