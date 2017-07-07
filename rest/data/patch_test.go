package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type PatchConnectorSuite struct {
	ctx   Connector
	time  time.Time
	setup func() error
	suite.Suite
}

func TestPatchConnectorSuite(t *testing.T) {
	s := new(PatchConnectorSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}
		s.time = time.Now()

		testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorSuite")
		db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

		patches := []*patch.Patch{
			{Id: "patch1", Project: "project1", CreateTime: s.time},
			{Description: "patch2", Project: "project2", CreateTime: s.time.Add(time.Minute * 2)},
			{Description: "patch3", Project: "project1", CreateTime: s.time.Add(time.Minute * 4)},
			{Description: "patch4", Project: "project1", CreateTime: s.time.Add(time.Minute * 6)},
			{Description: "patch5", Project: "project2", CreateTime: s.time.Add(time.Minute * 8)},
			{Description: "patch6", Project: "project1", CreateTime: s.time.Add(time.Minute * 10)},
		}

		for _, p := range patches {
			if err := p.Insert(); err != nil {
				return err
			}
		}

		return nil
	}

	suite.Run(t, s)
}

func TestMockPatchConnectorSuite(t *testing.T) {
	s := new(PatchConnectorSuite)
	s.setup = func() error {
		s.time = time.Now().UTC()
		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []patch.Patch{
				{Id: "patch1", Project: "project1", CreateTime: s.time},
				{Id: "patch2", Project: "project2", CreateTime: s.time.Add(time.Second * 2)},
				{Id: "patch3", Project: "project1", CreateTime: s.time.Add(time.Second * 4)},
				{Id: "patch4", Project: "project1", CreateTime: s.time.Add(time.Second * 6)},
				{Id: "patch5", Project: "project2", CreateTime: s.time.Add(time.Second * 8)},
				{Id: "patch6", Project: "project1", CreateTime: s.time.Add(time.Second * 10)},
			},
		}}

		return nil
	}

	suite.Run(t, s)
}

func (s *PatchConnectorSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *PatchConnectorSuite) TestFetchTooManyAsc() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time, 3, 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("project2", patches[0].Project)
	s.Equal("project2", patches[1].Project)
	s.True(patches[0].CreateTime.Before(patches[1].CreateTime))
}

func (s *PatchConnectorSuite) TestFetchTooManyDesc() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time.Add(time.Hour), 3, -1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("project2", patches[0].Project)
	s.Equal("project2", patches[1].Project)
	s.True(patches[0].CreateTime.After(patches[1].CreateTime))
}

func (s *PatchConnectorSuite) TestFetchExactNumber() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time, 1, 1)
	s.NoError(err)
	s.NotNil(patches)

	s.Len(patches, 1)
	s.Equal("project2", patches[0].Project)
}

func (s *PatchConnectorSuite) TestFetchTooFewAsc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time, 1, -1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time, patches[0].CreateTime)
}

func (s *PatchConnectorSuite) TestFetchTooFewDesc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Hour), 1, -1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*10), patches[0].CreateTime)
}

func (s *PatchConnectorSuite) TestFetchNonexistentFail() {
	patches, err := s.ctx.FindPatchesByProject("project3", s.time, 1, 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorSuite) TestFetchKeyWithinBoundAsc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second), 1, 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*4), patches[0].CreateTime)
}

func (s *PatchConnectorSuite) TestFetchKeyWithinBoundDesc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second), 1, -1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time, patches[0].CreateTime)
}

func (s *PatchConnectorSuite) TestFetchKeyOutOfBoundAsc() {
	patches, err := s.ctx.FindPatchesByProject("project3", s.time.Add(time.Hour), 1, 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorSuite) TestFetchKeyOutOfBoundDesc() {
	patches, err := s.ctx.FindPatchesByProject("project3", s.time, 1, -1)
	s.NoError(err)
	s.Len(patches, 0)
}
