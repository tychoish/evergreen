package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type repotrackerJobSuite struct {
	suite.Suite
	cancel func()
}

func TestRepotrackerJob(t *testing.T) {
	suite.Run(t, new(repotrackerJobSuite))
}

func (s *repotrackerJobSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	testutil.SetGlobalEnvironment(ctx, s.T())
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
}

func (s *repotrackerJobSuite) TearDownTest() {
	s.NoError(db.ClearCollections(evergreen.ConfigCollection))
	s.cancel()
}

func (s *repotrackerJobSuite) TestJob() {
	j := NewRepotrackerJob("1", "mci").(*repotrackerJob)
	s.Equal("mci", j.ProjectID)
	s.Equal("repotracker:1:mci", j.ID())
	j.Run(context.Background())
	s.Error(j.Error())
	s.Contains(j.Error().Error(), "can't find project ref for project")
	s.True(j.Status().Completed)
}

func (s *repotrackerJobSuite) TestRunFailsInDegradedMode() {
	flags := evergreen.ServiceFlags{
		RepotrackerDisabled: true,
	}
	s.NoError(evergreen.SetServiceFlags(flags))

	job := NewRepotrackerJob("1", "mci")
	job.Run(context.Background())

	s.Error(job.Error())
	s.Contains(job.Error().Error(), "repotracker is disabled")
}
