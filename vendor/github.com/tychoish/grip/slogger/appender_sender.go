package slogger

import (
	"fmt"
	"os"

	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
	"github.com/tychoish/grip/send"
)

///////////////////////////////////////////////////////////////////////////
//
// A shim between slogger.Append and send.Sender
//
///////////////////////////////////////////////////////////////////////////

type appenderSender struct {
	appender Appender
	name     string
	level    send.LevelInfo
}

// NewAppenderSender implements the send.Sender interface, which
// allows it to be used as a grip backend, but the it's mode of action
// is to use a slogger.Appender. This allows using the grip package,
// either via the slogger interface or the normal grip Jouernaler
// interface, while continuing to use existing slogger code.
func NewAppenderSender(name string, a Appender) send.Sender {
	return &appenderSender{
		appender: a,
		name:     name,
		level:    send.LevelInfo{level.Debug, level.Debug},
	}
}

// WrapAppender takes an Appender instance and returns a send.Sender
// instance that wraps it. The name defaults to the name of the
// process (argc).
func WrapAppender(a Appender) send.Sender {
	return &appenderSender{
		appender: a,
		name:     os.Args[0],
		level:    send.LevelInfo{level.Debug, level.Debug},
	}
}

// TODO: we may want to add a mutex here
func (a *appenderSender) Close() error          { return nil }
func (a *appenderSender) Name() string          { return a.name }
func (a *appenderSender) SetName(n string)      { a.name = n }
func (a *appenderSender) Type() send.SenderType { return send.Custom }
func (a *appenderSender) Level() send.LevelInfo { return a.level }
func (a *appenderSender) SetLevel(l send.LevelInfo) error {
	if !l.Valid() {
		return fmt.Errorf("level settings are not valid: %+v", l)
	}

	a.level = l
	return nil
}

func (a *appenderSender) Send(m message.Composer) {
	if a.level.ShouldLog(m) {
		_ = a.appender.Append(NewLog(m))
	}
}
