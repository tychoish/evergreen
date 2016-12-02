package evergreen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	slogger "github.com/10gen-labs/slogger/v1"
)

const (
	User = "mci"

	TestDir      = "config_test"
	TestSettings = "evg_settings.yml"

	HostRunning         = "running"
	HostTerminated      = "terminated"
	HostUninitialized   = "starting"
	HostInitializing    = "provisioning"
	HostProvisionFailed = "provision failed"
	HostUnreachable     = "unreachable"
	HostQuarantined     = "quarantined"
	HostDecommissioned  = "decommissioned"

	HostStatusSuccess = "success"
	HostStatusFailed  = "failed"

	TaskStarted      = "started"
	TaskUndispatched = "undispatched"
	TaskDispatched   = "dispatched"
	TaskFailed       = "failed"
	TaskSucceeded    = "success"
	TaskInactive     = "inactive"

	TestFailedStatus    = "fail"
	TestSkippedStatus   = "skip"
	TestSucceededStatus = "pass"

	BuildStarted   = "started"
	BuildCreated   = "created"
	BuildFailed    = "failed"
	BuildSucceeded = "success"

	VersionStarted   = "started"
	VersionCreated   = "created"
	VersionFailed    = "failed"
	VersionSucceeded = "success"

	PatchCreated   = "created"
	PatchStarted   = "started"
	PatchSucceeded = "succeeded"
	PatchFailed    = "failed"

	PushLogPushing = "pushing"
	PushLogSuccess = "success"

	HostTypeStatic = "static"

	CompileStage = "compile"
	TestStage    = "single_test"
	SanityStage  = "smokeCppUnitTests"
	PushStage    = "push"

	// maximum task (zero based) execution number
	MaxTaskExecution = 3

	// LogMessage struct versions
	LogmessageFormatTimestamp = 1
	LogmessageCurrentVersion  = LogmessageFormatTimestamp

	EvergreenHome = "EVGHOME"

	DefaultTaskActivator   = ""
	APIServerTaskActivator = "apiserver"

	AgentAPIVersion = 2
)

// evergreen package names
const (
	UIPackage = "EVERGREEN_UI"
)

const (
	AuthTokenCookie  = "mci-token"
	TaskSecretHeader = "Task-Secret"
)

var (
	// UphostStatus is a list of all host statuses that are considered "up."
	// This is used for query building.
	UphostStatus = []string{
		HostRunning,
		HostUninitialized,
		HostInitializing,
		HostProvisionFailed,
	}

	// Logger is our global logger. It can be changed for testing.
	Logger = slogger.Logger{
		Prefix:    "",
		Appenders: []slogger.Appender{slogger.StdOutAppender()},
	}

	// database and config directory, set to the testing version by default for safety
	NotificationsFile = "mci-notifications.yml"

	// version requester types
	PatchVersionRequester       = "patch_request"
	RepotrackerVersionRequester = "gitter_request"

	// constant arrays for db update logic
	AbortableStatuses = []string{TaskStarted, TaskDispatched}
	CompletedStatuses = []string{TaskSucceeded, TaskFailed}
)

// SetLogger sets the global logger to write to the given path.
func SetLogger(logPath string) {
	logfile, err := GetAppendingFile(logPath)
	if err != nil {
		panic(fmt.Sprintf("Cannot create log file %v: %v", logPath, err))
	}

	Logger = slogger.Logger{
		Prefix:    "",
		Appenders: []slogger.Appender{&slogger.FileAppender{logfile}},
	}
}

// GetAppendingFile opens a file for appending. The file will be created
// if it does not already exist.
func GetAppendingFile(path string) (*os.File, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := os.Create(path); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
}

// FindEvergreenHome finds the directory of the EVGHOME environment variable.
func FindEvergreenHome() string {
	// check if env var is set
	root := os.Getenv("EVGHOME")
	if len(root) > 0 {
		return root
	}
	return ""
}

// FindConfig finds the config root in the home directory.
// Returns an error if the root cannot be found or EVGHOME is unset.
func FindConfig(configName string) (string, error) {
	home := FindEvergreenHome()
	if len(home) > 0 {
		root, yes := isConfigRoot(home, configName)
		if yes {
			return root, nil
		}
		return "", fmt.Errorf("Can't find evergreen config root: '%v'", root)
	}
	return "", fmt.Errorf("%v environment variable must be set", EvergreenHome)
}

func isConfigRoot(home string, configName string) (fixed string, is bool) {
	fixed = filepath.Join(home, configName)
	fixed = strings.Replace(fixed, "\\", "/", -1)
	stat, err := os.Stat(fixed)
	if err == nil && stat.IsDir() {
		is = true
	}
	return
}

// TestConfig creates test settings from a test config.
func TestConfig() *Settings {
	evgHome := FindEvergreenHome()
	file := filepath.Join(evgHome, TestDir, TestSettings)
	settings, err := NewSettings(file)
	if err != nil {
		panic(err)
	}
	return settings
}

// IsSystemActivator returns true when the task activator is Evergreen.
func IsSystemActivator(caller string) bool {
	return caller == DefaultTaskActivator ||
		caller == APIServerTaskActivator
}
