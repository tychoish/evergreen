package servicecontext

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type DBMetricsConnector struct{}

func (mc *DBMetricsConnector) FindTaskSystemMetrics(taskId string, ts time.Time, limit, sort int) ([]*message.SystemInfo, error) {
	events := []event.Event{}
	out := []*message.SystemInfo{}

	db.FindAllQ(event.TaskLogCollection, event.TaskSystemInfoEvents(taskId, ts, limit, sort), &events)
	for _, e := range events {
		w, ok := e.Data.Data.(event.TaskSystemResourceData)
		if !ok {
			return nil, errors.Errorf("system resource event for task %s is malformed (of type %T)",
				taskId, e.Data)
		}
		out = append(out, w.SystemInfo)
	}

	return out, nil
}

func (mc *DBMetricsConnector) FindTaskProcessMetrics(taskId string, ts time.Time, limit, sort int) ([][]*message.ProcessInfo, error) {
	events := []event.Event{}
	out := [][]*message.ProcessInfo{}
	err := db.FindAllQ(event.TaskLogCollection, event.TaskProcessInfoEvents(taskId, ts, limit, sort), &events)
	grip.Alert(err)
	grip.Infof("%T: %+v", events, events)
	for _, e := range events {
		w, ok := e.Data.Data.(event.TaskProcessResourceData)
		if !ok {
			return nil, errors.Errorf("process resource event for task %s is malformed (of type %T)",
				taskId, e.Data)
		}
		grip.Info(w.Processes)

		out = append(out, w.Processes)
	}

	return out, nil
}

type MockMetricsConnector struct{}

func (mc *MockMetricsConnector) FindTaskSystemMetrics(taskId string, ts time.Time, limit, sort int) ([]*message.SystemInfo, error) {
	return []*message.SystemInfo{}, nil
}
func (mc *MockMetricsConnector) FindTaskProcessMetrics(taskId string, ts time.Time, limit, sort int) ([][]*message.ProcessInfo, error) {
	return [][]*message.ProcessInfo{}, nil
}
