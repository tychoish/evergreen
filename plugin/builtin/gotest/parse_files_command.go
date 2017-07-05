package gotest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
)

// ParseFilesCommand is a struct implementing plugin.Command. It is used to parse a file or
// series of files containing the output of go tests, and send the results back to the server.
type ParseFilesCommand struct {
	// a list of filename blobs to include
	// e.g. "monitor.suite", "output/*"
	Files []string `mapstructure:"files" plugin:"expand"`
}

// Name returns the string name for the parse files command.
func (c *ParseFilesCommand) Name() string {
	return ParseFilesCommandName
}

func (c *ParseFilesCommand) Plugin() string {
	return GotestPluginName
}

// ParseParams reads the specified map of parameters into the ParseFilesCommand struct, and
// validates that at least one file pattern is specified.
func (c *ParseFilesCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%s' params", c.Name())
	}

	if len(c.Files) == 0 {
		return errors.Errorf("error validating params: must specify at least one "+
			"file pattern to parse: '%+v'", params)
	}
	return nil
}

// Execute parses the specified output files and sends the test results found in them
// back to the server.
func (c *ParseFilesCommand) Execute(ctx context.Context, client client.Communicator, conf *model.TaskConfig) error {
	logger := client.GetLoggerProducer(conf.Task.Id, conf.Task.Secret)

	if err := plugin.ExpandValues(c, conf.Expansions); err != nil {
		err = errors.Wrap(err, "error expanding params")
		logger.Task().Error("Error parsing gotest files: %+v", err)
		return err
	}

	// make sure the file patterns are relative to the task's working directory
	for idx, file := range c.Files {
		c.Files[idx] = filepath.Join(conf.WorkDir, file)
	}

	// will be all files containing test results
	outputFiles, err := c.AllOutputFiles()
	if err != nil {
		return errors.Wrap(err, "error obtaining names of output files")
	}

	// make sure we're parsing something
	if len(outputFiles) == 0 {
		return errors.New("no files found to be parsed")
	}

	// parse all of the files
	logs, results, err := ParseTestOutputFiles(ctx, logger, conf, outputFiles)
	if err != nil {
		return errors.Wrap(err, "error parsing output results")
	}

	// ship all of the test logs off to the server
	pluginLogger.LogTask(slogger.INFO, "Sending test logs to server...")
	allResults := []*TestResult{}
	for idx, log := range logs {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		var logId string

		logId, err = client.SendTestLog(ctx, conf.Task.Id, conf.Task.Secret&log)
		if err != nil {
			// continue on error to let the other logs be posted
			logger.Task().Errorf("problem posting log: %v", err)
		}

		// add all of the test results that correspond to that log to the
		// full list of results
		for _, result := range results[idx] {
			result.LogId = logId
			allResults = append(allResults, result)
		}

	}
	logger.Task().Info("Finished posting logs to server")

	// convert everything
	resultsAsModel := ToModelTestResults(conf.Task, allResults)

	// ship the parsed results off to the server
	logger.Task().Task("Sending parsed results to server...")

	if err := cleint.SendTaskResults(ctx, conf.Task.Id, conf.Task.Secret, &resultsAsModel); err != nil {
		return errors.Wrap(err, "error posting parsed results to server")
	}
	logger.Task().Info("Successfully sent parsed results to server")

	return nil

}

// AllOutputFiles creates a list of all test output files that will be parsed, by expanding
// all of the file patterns specified to the command.
func (c *ParseFilesCommand) AllOutputFiles() ([]string, error) {

	outputFiles := []string{}

	// walk through all specified file patterns
	for _, pattern := range c.Files {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, errors.Wrap(err, "error expanding file patterns")
		}
		outputFiles = append(outputFiles, matches...)
	}

	// uniquify the list
	asSet := map[string]bool{}
	for _, file := range outputFiles {
		asSet[file] = true
	}
	outputFiles = []string{}
	for file := range asSet {
		outputFiles = append(outputFiles, file)
	}

	return outputFiles, nil

}

// ParseTestOutputFiles parses all of the files that are passed in, and returns the
// test logs and test results found within.
func ParseTestOutputFiles(ctx context.Context, logger client.Communicator,
	conf *model.TaskConfig, outputFiles []string) ([]model.TestLog, [][]*TestResult, error) {

	var results [][]*TestResult
	var logs []model.TestLog

	// now, open all the files, and parse the test results
	for _, outputFile := range outputFiles {
		if ctx.Err() != nil {
			return nil, nil, errors.New("command was stopped")
		}

		// assume that the name of the file, stripping off the ".suite" extension if present,
		// is the name of the suite being tested
		_, suiteName := filepath.Split(outputFile)
		suiteName = strings.TrimSuffix(suiteName, ".suite")

		// open the file
		fileReader, err := os.Open(outputFile)
		if err != nil {
			// don't bomb out on a single bad file
			logger.Task().Errorf("Unable to open file '%s' for parsing: %v",
				outputFile, err)
			continue
		}
		defer fileReader.Close()

		// parse the output logs
		parser := &VanillaParser{Suite: suiteName}
		if err := parser.Parse(fileReader); err != nil {
			// continue on error
			logger.Task().Error("Error parsing file '%s': %v", outputFile, err)
			continue
		}

		// build up the test logs
		logLines := parser.Logs()
		testLog := model.TestLog{
			Name:          suiteName,
			Task:          conf.Task.Id,
			TaskExecution: conf.Task.Execution,
			Lines:         logLines,
		}
		// save the results
		results = append(results, parser.Results())
		logs = append(logs, testLog)

	}
	return logs, results, nil
}

// ToModelTestResults converts the implementation of TestResults native
// to the gotest plugin to the implementation used by MCI tasks
func ToModelTestResults(_ *task.Task, results []*TestResult) task.TestResults {
	var modelResults []task.TestResult
	for _, res := range results {
		// start and end are times that we don't know,
		// represented as a 64bit floating point (epoch time fraction)
		var start float64 = float64(time.Now().Unix())
		var end float64 = start + res.RunTime.Seconds()
		var status string
		switch res.Status {
		// as long as we use a regex, it should be impossible to
		// get an incorrect status code
		case PASS:
			status = evergreen.TestSucceededStatus
		case SKIP:
			status = evergreen.TestSkippedStatus
		case FAIL:
			status = evergreen.TestFailedStatus
		}
		convertedResult := task.TestResult{
			TestFile:  res.Name,
			Status:    status,
			StartTime: start,
			EndTime:   end,
			LineNum:   res.StartLine - 1,
			LogId:     res.LogId,
		}
		modelResults = append(modelResults, convertedResult)
	}
	return task.TestResults{modelResults}
}
