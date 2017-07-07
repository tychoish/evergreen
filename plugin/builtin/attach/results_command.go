package attach

import (
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// AttachResultsCommand is used to attach MCI test results in json
// format to the task page.
type AttachResultsCommand struct {
	// FileLoc describes the relative path of the file to be sent.
	// Note that this can also be described via expansions.
	FileLoc string `mapstructure:"file_location" plugin:"expand"`
}

func (c *AttachResultsCommand) Name() string {
	return AttachResultsCmd
}

func (c *AttachResultsCommand) Plugin() string {
	return AttachPluginName
}

// ParseParams decodes the S3 push command parameters that are
// specified as part of an AttachPlugin command; this is required
// to satisfy the 'Command' interface
func (c *AttachResultsCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", c.Name())
	}

	if err := c.validateAttachResultsParams(); err != nil {
		return errors.Wrapf(err, "error validating '%v' params", c.Name())
	}
	return nil
}

// validateAttachResultsParams is a helper function that ensures all
// the fields necessary for attaching a results are present
func (c *AttachResultsCommand) validateAttachResultsParams() (err error) {
	if c.FileLoc == "" {
		return errors.New("file_location cannot be blank")
	}
	return nil
}

func (c *AttachResultsCommand) expandAttachResultsParams(
	taskConfig *model.TaskConfig) (err error) {
	c.FileLoc, err = taskConfig.Expansions.ExpandString(c.FileLoc)
	if err != nil {
		return errors.Wrap(err, "error expanding file_location")
	}
	return nil
}

// Execute carries out the AttachResultsCommand command - this is required
// to satisfy the 'Command' interface
func (c *AttachResultsCommand) Execute(ctx context.Context, comm client.Communicator, conf *model.TaskConfig) error {
	if err := c.expandAttachResultsParams(conf); err != nil {
		return errors.WithStack(err)
	}

	logger := comm.GetLoggerProducer(client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	errChan := make(chan error)
	go func() {
		reportFileLoc := c.FileLoc
		if !filepath.IsAbs(c.FileLoc) {
			reportFileLoc = filepath.Join(conf.WorkDir, c.FileLoc)
		}

		// attempt to open the file
		reportFile, err := os.Open(reportFileLoc)
		if err != nil {
			errChan <- errors.Wrapf(err, "Couldn't open report file '%s'", reportFileLoc)
			return
		}

		results := &task.TestResults{}
		if err = util.ReadJSONInto(reportFile, results); err != nil {
			errChan <- errors.Wrapf(err, "Couldn't read report file '%s'", reportFileLoc)
			return
		}
		if err := reportFile.Close(); err != nil {
			logger.Execution().Infof("Error closing file: %v", err)
		}

		errChan <- errors.WithStack(sendJSONResults(ctx, conf, logger, comm, results))
	}()

	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Info("Received signal to terminate execution of attach results command")
		return nil
	}
}

// SendJSONResults is responsible for sending the
// specified file to the API Server
func sendJSONResults(ctx context.Context, conf *model.TaskConfig,
	logger client.LoggerProducer, comm client.Communicator,
	results *task.TestResults) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	for i, res := range results.Results {
		if ctx.Err() != nil {
			return errors.Errorf("operation canceled after uploading ")
		}

		if res.LogRaw != "" {
			logger.Execution().Info("Attaching raw test logs")
			testLogs := &model.TestLog{
				Name:          res.TestFile,
				Task:          conf.Task.Id,
				TaskExecution: conf.Task.Execution,
				Lines:         []string{res.LogRaw},
			}

			id, err := comm.SendTestLog(ctx, td, testLogs)
			if err != nil {
				logger.Execution().Errorf("problem posting raw logs from results %s", err.Error())
			} else {
				results.Results[i].LogId = id
			}

			// clear the logs from the TestResult struct after it has been saved in the test logs. Since they are
			// being saved in the test_logs collection, we can clear them to prevent them from being saved in the task
			// collection.
			results.Results[i].LogRaw = ""
		}
	}
	logger.Execution().Info("attaching test results")

	err := comm.SendTaskResults(ctx, results)
	if err != nil {
		return errors.WithStack(err)
	}

	logger.Task().Info("Attach test results succeeded")

	return nil
}
