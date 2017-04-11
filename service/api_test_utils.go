package service

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/mongodb/grip"
)

type TestServer struct {
	URL string
	net.Listener
	*APIServer
	ts *httptest.Server
}

func (s *TestServer) Close() {
	grip.Alertln("closing test server:", s.URL)

	grip.CatchError(s.Listener.Close())
	s.ts.CloseClientConnections()
	s.ts.Close()
}

func CreateTestServer(settings *evergreen.Settings, tlsConfig *tls.Config, plugins []plugin.APIPlugin) (*TestServer, error) {
	port := testutil.NextPort()
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		return nil, err
	}

	apiServer, err := NewAPIServer(settings, plugins)
	apiServer.UserManager = testutil.MockUserManager{}
	if err != nil {
		return nil, err
	}
	var l net.Listener
	protocol := "http"

	h, err := apiServer.Handler()
	if err != nil {
		return nil, err
	}
	server := httptest.NewUnstartedServer(h)
	server.TLS = tlsConfig

	// We're not running ssl tests with the agent in any cases,
	// but currently its set up to clients of this test server
	// should figure out the port from the TestServer instance's
	// URL field.
	//
	// We try and make sure that the SSL servers on different
	// ports than their non-ssl servers.

	var addr string
	if tlsConfig == nil {
		addr = fmt.Sprintf(":%d", port)
		l, err = GetListener(addr)
		if err != nil {
			return nil, err
		}
		server.Listener = l
		go server.Start()
	} else {
		addr = fmt.Sprintf(":%d", port+1)
		l, err = GetTLSListener(addr, tlsConfig)
		if err != nil {
			return nil, err
		}
		protocol = "https"
		server.Listener = l
		go server.StartTLS()
	}

	ts := &TestServer{
		URL:       fmt.Sprintf("%s://localhost%v", protocol, addr),
		Listener:  l,
		APIServer: apiServer,
		ts:        server,
	}

	grip.Infoln("started server:", ts.URL)

	return ts, nil
}
