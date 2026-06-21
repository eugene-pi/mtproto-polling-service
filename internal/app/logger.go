package app

import (
	"log"

	"github.com/kardianos/service"
)

// logger adapts kardianos' service.Logger to the proxy.Logger interface used by
// the rest of the app. When running interactively the service logger writes to
// stderr; when running as a Windows service it writes to the Event Log.
type logger struct {
	svc service.Logger
}

func newLogger(svc service.Logger) *logger { return &logger{svc: svc} }

func (l *logger) Infof(format string, args ...any) {
	if l.svc != nil {
		_ = l.svc.Infof(format, args...)
		return
	}
	log.Printf("INFO  "+format, args...)
}

func (l *logger) Warnf(format string, args ...any) {
	if l.svc != nil {
		_ = l.svc.Warningf(format, args...)
		return
	}
	log.Printf("WARN  "+format, args...)
}

func (l *logger) Errorf(format string, args ...any) {
	if l.svc != nil {
		_ = l.svc.Errorf(format, args...)
		return
	}
	log.Printf("ERROR "+format, args...)
}
