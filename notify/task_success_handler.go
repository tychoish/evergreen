package notify

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/tychoish/grip"
)

// Handler for notifying users that a task succeeded. Implements
// NotificationHandler from notification_handler.go.
type TaskSuccessHandler struct {
	TaskNotificationHandler
	Name string
}

func (self *TaskSuccessHandler) GetNotifications(ae *web.App, configName string,
	key *NotificationKey) ([]Email, error) {
	var emails []Email
	preface := mciSuccessPreface
	if key.NotificationRequester == evergreen.PatchVersionRequester {
		preface = patchSuccessPreface
	}
	triggeredNotifications, err :=
		self.getRecentlyFinishedTasksWithStatus(key, evergreen.TaskSucceeded, preface, successSubject)

	if err != nil {
		return nil, err
	}

	for _, triggered := range triggeredNotifications {
		email, err := self.TemplateNotification(ae, configName, &triggered)
		if err != nil {
			grip.Warningf("Error templating notification for task '%s': %+v",
				triggered.Current.Id, err)
			continue
		}

		emails = append(emails, email)
	}

	return emails, nil
}

func (self *TaskSuccessHandler) TemplateNotification(ae *web.App, configName string,
	notification *TriggeredTaskNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, configName, notification, changeInfo)
}

func (self *TaskSuccessHandler) GetChangeInfo(
	notification *TriggeredTaskNotification) ([]ChangeInfo, error) {
	return self.constructChangeInfo([]task.Task{*notification.Current}, &notification.Key)
}
