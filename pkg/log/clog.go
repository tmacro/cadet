package log

import (
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

type ClogShim struct {
	clog clog.P
}

func NewClogShim(c clog.P) *ClogShim {
	return &ClogShim{
		clog: c,
	}
}

var _ API = (*ClogShim)(nil)

func (c *ClogShim) Info(v ...any) {
	c.clog.Info(v...)
}

func (c *ClogShim) Infof(format string, v ...any) {
	c.clog.Infof(format, v...)
}

func (c *ClogShim) Error(v ...any) {
	c.clog.Error(v...)
}

func (c *ClogShim) Errorf(format string, v ...any) {
	c.clog.Errorf(format, v...)
}

func (c *ClogShim) Warn(v ...any) {
	c.clog.Warning(v...)
}

func (c *ClogShim) Warnf(format string, v ...any) {
	c.clog.Warningf(format, v...)
}

func (c *ClogShim) Fatal(v ...any) {
	c.clog.Fatal(v...)
}

func (c *ClogShim) Fatalf(format string, v ...any) {
	c.clog.Fatalf(format, v...)
}

func (c *ClogShim) Debug(v ...any) {
	c.clog.Debug(v...)
}

func (c *ClogShim) Debugf(format string, v ...any) {
	c.clog.Debugf(format, v...)
}
