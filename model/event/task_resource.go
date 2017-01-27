package event

import (
	"time"

	"github.com/tychoish/grip"
	"github.com/tychoish/grip/message"
)

const (
	ResourceTypeTaskInfo = "RESOURCE_INFO"
	EventTaskSystemInfo  = "TASK_SYSTEM_INFO"
	EventTaskProcessInfo = "TASK_PROCESS_INFO"
)

type TaskSystemResourceData struct {
	ResourceType string `bson:"r_type" json:"resource_type"`
	*message.SystemInfo
}

func (d TaskSystemResourceData) IsValid() bool {
	return d.ResourceType == EventTaskSystemInfo
}

func LogTaskSystemData(taskId string, info *message.SystemInfo) {
	event := Event{
		ResourceId: taskId,
		Timestamp:  info.Base.Time,
		EventType:  EventTaskSystemInfo,
	}

	info.Base = message.Base{}
	data := TaskSystemResourceData{
		ResourceType: ResourceTypeTaskInfo,
		SystemInfo:   info,
	}
	event.Data = DataWrapper{data}

	if err := NewDBEventLogger(TaskCollection).LogEvent(event); err != nil {
		grip.Error(message.NewErrorWrap(err, "problem system info event"))
	}
}

type TaskProcessResourceData struct {
	ResourceType string                 `bson:"r_type" json:"resource_type"`
	Processes    []*message.ProcessInfo `bson:"processes" json:"processes"`
}

func (d TaskProcessResourceData) IsValid() bool {
	return d.ResourceType == EventTaskProcessInfo
}

func LogTaskProcessData(taskId string, procs []*message.ProcessInfo) {
	ts := time.Now()
	b := message.Base{}
	for _, p := range procs {
		if p.Parent == 0 {
			ts = p.Base.Time
		}

		p.Base = b
	}

	data := TaskProcessResourceData{
		ResourceType: ResourceTypeTaskInfo,
		Processes:    proces,
	}

	event := Event{
		Timestamp:  ts,
		ResourceId: taskId,
		EventType:  EventTaskProcessInfo,
		Data:       DataWrapper{data},
	}

	if err := NewDBEventLogger(TaskCollection).LogEvent(event); err != nil {
		grip.Error(message.NewErrorWrap(err, "problem logging task process info event"))
	}
}
