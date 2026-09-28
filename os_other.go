//go:build !windows

package main

import "errors"

func setAutoStart(bool) error { return errors.New("开机自启仅支持 Windows") }
