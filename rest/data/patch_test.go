package data

import (
	"testing"

	"gopkg.in/mgo.v2/bson"

	"time"

	"fmt"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PatchConnectorSuite struct {
	ctx  Connector
	time time.Time
	suite.Suite
}

func TestPatchConnectorSuite(t *testing.T) {
	s := new(PatchConnectorSuite)
	s.ctx = &DBConnector{}
	//s.time = time.Now()
	s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)

	testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	patch1 := &patch.Patch{Id: "patch1", Project: "project1", CreateTime: s.time}
	patch2 := &patch.Patch{Id: "patch2", Project: "project2", CreateTime: s.time.Add(time.Second * 2)}
	patch3 := &patch.Patch{Id: "patch3", Project: "project1", CreateTime: s.time.Add(time.Second * 4)}
	patch4 := &patch.Patch{Id: "patch4", Project: "project1", CreateTime: s.time.Add(time.Second * 6)}
	patch5 := &patch.Patch{Id: "patch5", Project: "project2", CreateTime: s.time.Add(time.Second * 8)}
	patch6 := &patch.Patch{Id: "patch6", Project: "project1", CreateTime: s.time.Add(time.Second * 10)}

	assert.NoError(t, patch1.Insert())
	assert.NoError(t, patch2.Insert())
	assert.NoError(t, patch3.Insert())
	assert.NoError(t, patch4.Insert())
	assert.NoError(t, patch5.Insert())
	assert.NoError(t, patch6.Insert())

	//bson.M{}
	p, _ := patch.Find(db.Query(bson.M{"create_time": bson.M{"$lte": s.time}}))
	//p, _ := patch.Find(db.Query(bson.M{"create_time": s.time}))

	fmt.Println(p)
	fmt.Println(s.time)
	suite.Run(t, s)
}

func TestMockPatchConnectorSuite(t *testing.T) {
	s := new(PatchConnectorSuite)
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
	suite.Run(t, s)
}

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
