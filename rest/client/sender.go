package client

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"golang.org/x/net/context"
)

type logSender struct {
	taskID     string
	taskSecret string
	logChannel string
	comm       Communicator
	*send.Base
}

func newLogSender(comm Communicator, channel, taskID, taskSecret string) send.Sender {
	return &logSender{
		comm:       comm,
		logChannel: channel,
		taskID:     taskID,
		taskSecret: taskSecret,
	}
}

func (s *logSender) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		err := s.comm.SendTaskLogMessages(
			context.TODO(),
			s.taskID,
			s.taskSecret,
			s.convertMessages(m))

		if err != nil {
			s.ErrorHandler(err, m)
		}
	}
}

func (s *logSender) convertMessages(m message.Composer) []apimodels.LogMessage {
	out := []apimodels.LogMessage{}
	switch m := m.(type) {
	case *message.GroupComposer:
		out = append(out, s.convertMessages(m)...)
	default:
		out = append(out, apimodels.LogMessage{
			Type:      s.logChannel,
			Severity:  priorityToString(m.Priority()),
			Message:   m.String(),
			Timestamp: time.Now(),
			Version:   evergreen.LogmessageCurrentVersion,
		})
	}
	return out
}

func priorityToString(l level.Priority) string {
	switch l {
	case level.Trace, level.Debug:
		return apimodels.LogDebugPrefix
	case level.Notice, level.Info:
		return apimodels.LogInfoPrefix
	case level.Warning:
		return apimodels.LogWarnPrefix
	case level.Error, level.Alert, level.Critical, level.Emergency:
		return apimodels.LogErrorPrefix
	default:
		return "UNKNOWN"
	}
}
