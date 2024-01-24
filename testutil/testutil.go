package testutil

import (
	"bytes"
)

type CmdOut struct {
	OutBuf *bytes.Buffer
	ErrBuf *bytes.Buffer
}

func (c CmdOut) String() string {
	return c.OutBuf.String()
}

func (c CmdOut) Stderr() string {
	return c.ErrBuf.String()
}
