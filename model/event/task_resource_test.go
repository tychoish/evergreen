package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/tychoish/grip/message"
)

func TestTaskInfoEvent(t *testing.T) {
	Convey("Test task resource utilization collection and persistence", t, func() {

		testutil.HandleTestingErr(db.Clear(TaskCollection), t,
			"Error clearing '%s' collection", TaskCollection)

		Convey("when logging system task info;", func() {
			taskId := "testId"

			Convey("before logging tasks, the query should not return any results", func() {
				results, err := FindTaskEvent(TaskSystemInfoEvents(taskId, 0))
				So(err, ShouldBeNil)
				So(len(results), ShouldEqual, 0)
			})

			Convey("logging a task should be retrievable,", func() {
				sysInfo, ok := message.CollectSystemInfo().(*message.SystemInfo)
				So(ok, ShouldBeTrue)

				LogTaskSystemData(taskId, sysInfo)
				results, err := FindTaskEvent(TaskSystemInfoEvents(taskId, 0))
				So(err, ShouldBeNil)
				So(len(results), ShouldEqual, 1)
			})

			Convey("when logging many tasks they're all retrievable", func() {
				taskId += "-batch"
				for i := 0; i < 10; i++ {
					info, ok := message.CollectSystemInfo().(*message.SystemInfo)
					So(ok, ShouldBeTrue)
					LogTaskSystemData(taskId, info)
				}

				results, err := FindTaskEvent(TaskSystemInfoEvents(taskId, 0))
				So(err, ShouldBeNil)
				So(len(results), ShouldEqual, 10)

			})
		})

		testutil.HandleTestingErr(db.Clear(TaskCollection), t,
			"Error clearing '%s' collection", TaskCollection)

		Convey("when logging process tree", func() {
			taskId := "taskId"
			Convey("before logging tasks, the query should not return any results", func() {
				results, err := FindTaskEvent(TaskProcessInfoEvents(taskId, 0))
				So(err, ShouldBeNil)
				So(len(results), ShouldEqual, 0)
			})

			Convey("log events should be retrievable", func() {
				pm, ok := message.CollectProcessInfoSelf().(*message.ProcessInfo)
				So(ok, ShouldBeTrue)

				LogTaskProcessData(taskId, []*message.ProcessInfo{pm})

				results, err := FindTaskEvent(TaskProcessInfoEvents(taskId, 0))
				So(err, ShouldBeNil)
				So(len(results), ShouldEqual, 1)
			})

			Convey("logging multiple events should be retrievable", func() {
				var count int
				taskId += "batch"

				infos := []*message.ProcessInfo{}
				msgs := message.CollectProcessInfoSelfWithChildren()

				for _, m := range msgs {
					count++

					info, ok := m.(*message.ProcessInfo)
					So(ok, ShouldBeTrue)
					infos = append(infos, info)
				}
				So(len(infos), ShouldEqual, len(msgs))
				LogTaskProcessData(taskId, infos)

				So(count, ShouldEqual, len(infos))
				results, err := FindTaskEvent(TaskProcessInfoEvents(taskId, 0))
				So(err, ShouldBeNil)
				So(len(results), ShouldEqual, count)
			})
		})
	})
}
