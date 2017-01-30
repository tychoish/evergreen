package service

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestResourceInfoEndPoints(t *testing.T) {
	testConfig := evergreen.TestConfig()
	testApiServer, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins, true)
	testutil.HandleTestingErr(err, t, "failed to create new API server")

	Convey("For the task resource use endpoints", t, func() {

	})
}
