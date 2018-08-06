package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UserConnectorSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler
	suite.Suite
}

func TestUserConnectorSuite(t *testing.T) {
	s := new(UserConnectorSuite)
	suite.Run(t, s)
}

func (s *UserConnectorSuite) SetupTest() {
	s.sc = &data.MockConnector{MockUserConnector: data.MockUserConnector{
		CachedUsers: map[string]*user.DBUser{
			"user0": {
				Id:     "user0",
				APIKey: "apikey0",
				PubKeys: []user.PubKey{
					{
						Name:      "user0_pubkey0",
						Key:       "ssh-mock 12345",
						CreatedAt: time.Now(),
					},
					{
						Name:      "user0_pubkey1",
						Key:       "ssh-mock 67890",
						CreatedAt: time.Now(),
					},
				},
			},
			"user1": {
				Id:     "user1",
				APIKey: "apikey1",
				// no pub keys
			},
		},
	}}
	s.rm = makeFetchKeys(s.sc)
}

func (s *UserConnectorSuite) TestGetSshKeysWithNoUserPanics() {
	s.PanicsWithValue("no user attached to request", func() {
		_ = s.rm.Run(context.TODO())
	})
}

func (s *UserConnectorSuite) TestGetSshKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	resp := s.rm.Run(ctx)

	s.Equal(http.StatusOK, resp.Status())
	payload := resp.Data().([]interface{})
	s.Len(payload, 2)
	for i, result := range payload {
		s.IsType(new(model.APIPubKey), result)
		key := result.(*model.APIPubKey)
		s.Equal(key.Name, model.ToAPIString(fmt.Sprintf("user0_pubkey%d", i)))
	}
}

func (s *UserConnectorSuite) TestGetSshKeysWithEmptyPubKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])

	resp := s.rm.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
}

func (s *UserConnectorSuite) TestAddSshKeyWithNoUserPanics() {
	s.rm.(*keysPostHandler).keyName = "Test"
	s.rm.(*keysPostHandler).keyValue = "ssh-rsa 12345"

	s.PanicsWithValue("no user attached to request", func() {
		_, _ = s.rm.Run(context.TODO())
	})
}

func (s *UserConnectorSuite) TestAddSshKey() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	s.rm.(*keysPostHandler).keyName = "Test"
	s.rm.(*keysPostHandler).keyValue = "ssh-dss 12345"
	resp := s.rm.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())

	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 3)
	s.Equal("Test", s.sc.MockUserConnector.CachedUsers["user0"].PubKeys[2].Name)
	s.Equal("ssh-dss 12345", s.sc.MockUserConnector.CachedUsers["user0"].PubKeys[2].Key)
}

func (s *UserConnectorSuite) TestAddDuplicateSshKeyFails() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])
	s.TestAddSshKey()

	data, err := s.rm.Methods[1].Execute(ctx, s.sc)

	s.Empty(data.Result)
	s.Error(err)

	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 3)
}

func TestKeyValidationFailsWithInvalidKeys(t *testing.T) {
	assert := assert.New(t)

	err := validateKeyName("    ")
	assert.Error(err)
	assert.Equal("empty key name", err.Error())

	err2 := validateKeyValue("    ")
	assert.Error(err2)
	assert.Equal("invalid public key", err2.Error())

	err3 := validateKeyValue("ssh-rsa notvalidbase64")
	assert.Error(err3)
	assert.Equal("invalid public key: key contents invalid", err3.Error())
}

func TestKeyValidation(t *testing.T) {
	assert := assert.New(t)

	err := validateKeyName("key1 ")
	assert.NoError(err)

	err2 := validateKeyValue("ssh-rsa YWJjZDEyMzQK")
	assert.NoError(err2)
}

type UserConnectorDeleteSuite struct {
	sc *data.MockConnector
	rm gimlet.RouteHandler
	suite.Suite
}

func (s *UserConnectorDeleteSuite) SetupTest() {
	s.rm = getKeysDeleteRouteManager("", 2)
	s.sc = &data.MockConnector{MockUserConnector: data.MockUserConnector{
		CachedUsers: map[string]*user.DBUser{
			"user0": {
				Id:     "user0",
				APIKey: "apikey0",
				PubKeys: []user.PubKey{
					{
						Name:      "user0_pubkey0",
						Key:       "ssh-mock 12345",
						CreatedAt: time.Now(),
					},
					{
						Name:      "user0_pubkey1",
						Key:       "ssh-mock 67890",
						CreatedAt: time.Now(),
					},
				},
			},
			"user1": {
				Id:     "user1",
				APIKey: "apikey1",
				// no pub keys
			},
		},
	}}
}

func (s *UserConnectorDeleteSuite) TestDeleteSshKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user0"])

	s.rm.Methods[0].RequestHandler.(*keysDeleteHandler).keyName = "user0_pubkey0"
	resp := s.rm.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	s.Len(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys, 1)

	s.rm.Methods[0].RequestHandler.(*keysDeleteHandler).keyName = "user0_pubkey1"
	data2, err2 := s.rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err2)
	s.Empty(data2.Result)
	s.Empty(s.sc.MockUserConnector.CachedUsers["user0"].PubKeys)
}

func (s *UserConnectorDeleteSuite) TestDeleteSshKeysWithEmptyPubKeys() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, s.sc.MockUserConnector.CachedUsers["user1"])

	s.rm.Methods[0].RequestHandler.(*keysDeleteHandler).keyName = "keythatdoesntexist"
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

func (s *UserConnectorDeleteSuite) TestDeleteSshKeysWithNoUserFails() {
	s.PanicsWithValue("no user attached to request", func() {
		_, _ = s.rm.Methods[0].Execute(context.TODO(), s.sc)
	})
}

func TestUserConnectorDeleteSuite(t *testing.T) {
	s := new(UserConnectorDeleteSuite)
	suite.Run(t, s)
}
