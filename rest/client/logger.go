package client

import (
	"io"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// LoggerProducer provides a mechanism for agents (and command pluings) to access the
// process' logging facilities. The interfaces are all based on grip
// interfaces and abstractions, and the behavior of the interfaces is
// dependent on the configuration and implementation of the
// LoggerProducer instance.
type LoggerProducer interface {
	// Provides access to the local logger. In most implementations
	// this is roughly equivalent to using the standard "grip" logger.
	Local() grip.Journaler

	// The Execution/Task/System loggers provide a grip-like
	// logging interface for the distinct logging channels that the
	// Evergreen agent provides to tasks
	Execution() grip.Journaler
	Task() grip.Journaler
	System() grip.Journaler

	// The writer functions return an io.Writer for use with
	// exec.Cmd operations for capturing standard output and standard
	// error from sbprocesses.
	TaskWriter() io.Writer
	SystemWriter() io.Writer

	// Close releases all resources by calling Close on all underlying senders.
	Close() error
}

////////////////////////////////////////////////////////////////////////
//
// Standard/Default Production  LoggerProducer

// LogHarness provides a straightforward implementation of the
// plugin.LoggerProducer interface.
type logHarness struct {
	local     grip.Journaler
	execution grip.Journaler
	task      grip.Journaler
	system    grip.Journaler

	mu      sync.Mutex
	writers []io.WriteCloser
}

// NewLogHanress takes a name, presumably of a task, and three
// senders for execution, task, and system logging channels. The
// "local" logging channel uses the same as the standard "grip"
// logging instance.
func NewLogHarness(name string, execution, task, system send.Sender) LoggerProducer {
	for _, s := range []send.Sender{execution, task, system} {
		s.SetName(name)
	}

	l := &logHarness{
		local:     &logging.Grip{Sender: grip.GetSender()},
		execution: &logging.Grip{Sender: execution},
		task:      &logging.Grip{Sender: task},
		system:    &logging.Grip{Sender: system},
	}

	return l
}

func (l *logHarness) Local() grip.Journaler     { return l.local }
func (l *logHarness) Execution() grip.Journaler { return l.execution }
func (l *logHarness) Task() grip.Journaler      { return l.task }
func (l *logHarness) System() grip.Journaler    { return l.system }

func (l *logHarness) TaskWriter() io.Writer {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := send.NewWriterSender(l.task.GetSender())
	l.writers = append(l.writers, w)
	return w
}

func (l *logHarness) SystemWriter() io.Writer {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := send.NewWriterSender(l.system.GetSender())
	l.writers = append(l.writers, w)
	return w
}

func (l *logHarness) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	catcher := grip.NewCatcher()

	for _, w := range l.writers {
		catcher.Add(w.Close())
	}

	catcher.Add(l.local.GetSender().Close())
	catcher.Add(l.execution.GetSender().Close())
	catcher.Add(l.task.GetSender().Close())
	catcher.Add(l.system.GetSender().Close())

	return errors.Wrap(catcher.Resolve(), "problem closing log harness")
}

////////////////////////////////////////////////////////////////////////
//
// Single Channel LoggerProducer

type singleChannelLogHarness struct {
	local   grip.Journaler
	logger  grip.Journaler
	mu      sync.Mutex
	writers []io.WriteCloser
}

// NewSingleChannelLogHarnness returns a log implementation that uses
// a LoggerProducer where Execution, Task, and System systems all use
// the same sender. The Local channel still wraps the default global
// sender.
//
// This implementation is primarily for testing and should be used
// with the InternalSender, which permits introspection of log messages.
func NewSingleChannelLogHarness(name string, sender send.Sender) LoggerProducer {
	sender.SetName(name)

	l := &singleChannelLogHarness{
		local:  &logging.Grip{Sender: grip.GetSender()},
		logger: &logging.Grip{Sender: sender},
	}

	return l
}

func (l *singleChannelLogHarness) Local() grip.Journaler     { return l.local }
func (l *singleChannelLogHarness) Execution() grip.Journaler { return l.logger }
func (l *singleChannelLogHarness) Task() grip.Journaler      { return l.logger }
func (l *singleChannelLogHarness) System() grip.Journaler    { return l.logger }

func (l *singleChannelLogHarness) TaskWriter() io.Writer {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := send.NewWriterSender(l.logger.GetSender())
	l.writers = append(l.writers, w)
	return w
}

func (l *singleChannelLogHarness) SystemWriter() io.Writer {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := send.NewWriterSender(l.logger.GetSender())
	l.writers = append(l.writers, w)
	return w
}

func (l *singleChannelLogHarness) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	catcher := grip.NewCatcher()

	for _, w := range l.writers {
		catcher.Add(w.Close())
	}

	catcher.Add(l.logger.GetSender().Close())

	return errors.Wrap(catcher.Resolve(), "problem closing log harness")
}
