package data

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type DBMetricsConnector struct{}

func (mc *DBMetricsConnector) FindTaskSystemMetrics(taskId string, ts time.Time, limit, sort int) ([]*message.SystemInfo, error) {
	out := []*message.SystemInfo{}
	events, err := event.Find(event.TaskLogCollection, event.TaskSystemInfoEvents(taskId, ts, limit, sort))
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching task system metrics for %s", taskId)
	}

	for _, e := range events {
		w, ok := e.Data.Data.(*event.TaskSystemResourceData)
		if !ok {
			return nil, errors.Errorf("system resource event for task %s is malformed (of type %T)",
				taskId, e.Data)
		}

		out = append(out, w.SystemInfo)
	}

	return out, nil
}

func (mc *DBMetricsConnector) FindTaskProcessMetrics(taskId string, ts time.Time, limit, sort int) ([][]*message.ProcessInfo, error) {
	out := [][]*message.ProcessInfo{}
	events, err := event.Find(event.TaskLogCollection, event.TaskProcessInfoEvents(taskId, ts, limit, sort))
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching task process metrics for %s", taskId)
	}

	for _, e := range events {
		w, ok := e.Data.Data.(*event.TaskProcessResourceData)
		if !ok {
			return nil, errors.Errorf("process resource event for task %s is malformed (of type %T)",
				taskId, e.Data)
		}

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
