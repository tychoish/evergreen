package notify

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/mongodb/grip"
)

// Handler for notifications generated specifically when a build fails and the
// previous finished build succeeded. Implements NotificationHandler from
// notification_handler.go.
type BuildSuccessToFailureHandler struct {
	BuildNotificationHandler
	Name string
}

func (self *BuildSuccessToFailureHandler) GetNotifications(ae *web.App, key *NotificationKey) ([]Email, error) {
	var emails []Email
	builds, err := getRecentlyFinishedBuilds(key)
	if err != nil {
		return nil, err
	}

	preface := mciFailurePreface
	if key.NotificationRequester == evergreen.PatchVersionRequester {
		preface = patchFailurePreface
	}

	for _, currentBuild := range builds {
		// Copy by value to make pointer safe
		curr := currentBuild
		previousBuild, err := currentBuild.PreviousActivated(key.Project,
			evergreen.RepotrackerVersionRequester)
		if previousBuild == nil {
			grip.Debugf("No previous completed build found for '%v' on %v %v notification",
				currentBuild.Id, key.Project, key.NotificationName)
			continue
		} else if err != nil {
			return nil, err
		}

		if !previousBuild.IsFinished() {
			grip.Debugf("Build before '%s' (on %s %s notification) isn't finished",
				currentBuild.Id, key.Project, key.NotificationName)
			continue
		}

		// get the build's project to add to the notification subject line
		branchName := UnknownProjectBranch
		if projectRef, err := getProjectRef(currentBuild.Project); err != nil {
			grip.Warningf("Unable to find project ref for build '%s': %+v",
				currentBuild.Id, err)
		} else if projectRef != nil {
			branchName = projectRef.Branch
		}

		grip.Debugf("Previous completed build found for '%s on %s %s notification is %s",
			currentBuild.Id, key.Project, key.NotificationName, previousBuild.Id)

		if previousBuild.Status == evergreen.BuildSucceeded &&
			currentBuild.Status == evergreen.BuildFailed {
			notification := TriggeredBuildNotification{
				Current:    &curr,
				Previous:   previousBuild,
				Key:        *key,
				Preface:    fmt.Sprintf(preface, branchName),
				Transition: transitionSubject,
			}
			email, err := self.TemplateNotification(ae, &notification)
			if err != nil {
				grip.Debugf("Error templating for build '%s': %+v", currentBuild.Id, err)
				continue
			}
			emails = append(emails, email)
		}
	}

	return emails, nil
}

func (self *BuildSuccessToFailureHandler) TemplateNotification(ae *web.App, notification *TriggeredBuildNotification) (Email, error) {
	changeInfo, err := self.GetChangeInfo(notification)
	if err != nil {
		return nil, err
	}
	return self.templateNotification(ae, notification, changeInfo)
}

func (self *BuildSuccessToFailureHandler) GetChangeInfo(
	notification *TriggeredBuildNotification) ([]ChangeInfo, error) {
	current := notification.Current
	previous := current
	if notification.Previous != nil {
		previous = notification.Previous
	}

	intermediateBuilds, err := current.FindIntermediateBuilds(previous)
	if err != nil {
		return nil, err
	}
	allBuilds := make([]build.Build, len(intermediateBuilds)+1)

	// include the current/previous build
	allBuilds[len(allBuilds)-1] = *current

	// copy any intermediate build(s)
	if len(intermediateBuilds) != 0 {
		copy(allBuilds[0:len(allBuilds)-1], intermediateBuilds)
	}
	return self.constructChangeInfo(allBuilds, &notification.Key)
}
