package testutil

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	util "github.com/evergreen-ci/evergreen/testutil"
	"gopkg.in/yaml.v2"
)

// path to an mci settings file containing sensitive information
var settingsOverride = flag.String("evergreen.settingsOverride", "", "Settings file"+
	" to be used to override sensitive info in the testing mci settings"+
	" file")

func ConfigureIntegrationTest(t *testing.T, testSettings *evergreen.Settings,
	testName string) {

	util.SkipTestUnlessAll(t, testName)

	// make sure an override file is provided
	if (*settingsOverride) == "" {
		msg := "Integration tests need a settings override file to be provided"
		keyName := "evergreen.settingsOverride"
		if !strings.Contains(os.Getenv("TEST_ARGS"), keyName) {
			panic(msg)
		}
		for _, k := range os.Environ() {
			if strings.HasPrefix(k, keyName) {
				parts := strings.Split(k, "=")
				if len(parts) < 2 {
					panic(msg)
				}
				*settingsOverride = parts[1]
			}
		}
	}

	// grab the file with the integration test settings
	integrationSettings, err := evergreen.NewSettings(*settingsOverride)
	if err != nil {
		panic(fmt.Sprintf("Error opening settings override file %v: %v",
			*settingsOverride, err))
	}

	// override the appropriate params
	t.Logf("Loading cloud provider settings from %v", *settingsOverride)

	testSettings.Providers = integrationSettings.Providers
	testSettings.Credentials = integrationSettings.Credentials
	testSettings.AuthConfig = integrationSettings.AuthConfig
	testSettings.Plugins = integrationSettings.Plugins
	testSettings.Jira = integrationSettings.Jira
}

// Creates a project ref local config that can be used for testing, with the string identifier given
// and the local config from a path
func CreateTestLocalConfig(testSettings *evergreen.Settings, projectName, projectPath string) error {

	if projectPath == "" {
		config, err := evergreen.FindConfig(testSettings.ConfigDir)
		if err != nil {
			return err
		}
		projectPath = filepath.Join(config, "project", fmt.Sprintf("%v.yml", projectName))
	}

	projectRef, err := model.FindOneProjectRef(projectName)
	if err != nil {
		return err
	}

	if projectRef == nil {
		projectRef = &model.ProjectRef{}
	}

	data, err := ioutil.ReadFile(projectPath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, projectRef)
	if err != nil {
		return err
	}

	projectRef.LocalConfig = string(data)

	return projectRef.Upsert()
}
