package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/plugin"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/send"
	"gopkg.in/tylerb/graceful.v1"
)

var (
	// requestTimeout is the duration to wait until killing
	// active requests and stopping the server.
	requestTimeout = 10 * time.Second
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s handles communication with running tasks and command line tools.\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Supported flags are:\n")
		flag.PrintDefaults()
	}
}

func main() {
	grip.SetName("api-server")

	go util.DumpStackOnSIGQUIT(os.Stdout)
	settings := evergreen.GetSettingsOrExit()

	// setup the logging
	if settings.Api.LogFile != "" {
		sender, err := send.MakeCallSiteFileLogger(settings.Api.LogFile, 2)
		grip.CatchEmergencyFatal(err)
		defer sender.Close()
		grip.CatchEmergencyFatal(grip.SetSender(sender))

		// for legacy compatibility. delete when this ticket is resolved
		evergreen.SetLogger(settings.Api.LogFile)
	}
	sender := send.MakeCallSiteConsoleLogger(2)
	defer sender.Close()
	grip.CatchEmergencyFatal(grip.SetSender())
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	tlsConfig, err := util.MakeTlsConfig(settings.Api.HttpsCert, settings.Api.HttpsKey)
	if err != nil {
		grip.EmergencyFatalf("Failed to make TLS config: %+v", err)
	}

	nonSSL, err := service.GetListener(settings.Api.HttpListenAddr)
	if err != nil {
		grip.EmergencyFatalf("Failed to get HTTP listener: %+v", err)
	}

	ssl, err := service.GetTLSListener(settings.Api.HttpsListenAddr, tlsConfig)
	if err != nil {
		grip.EmergencyFatalf("Failed to get HTTPS listener: %+v", err)
	}

	// Start SSL and non-SSL servers in independent goroutines, but exit
	// the process if either one fails
	as, err := service.NewAPIServer(settings, plugin.APIPlugins)
	if err != nil {
		grip.EmergencyFatalf("Failed to create API server: %+v", err)
	}

	handler, err := as.Handler()
	if err != nil {
		grip.EmergencyFatalf("Failed to get API route handlers: %+v", err)
	}

	server := &http.Server{Handler: handler}

	errChan := make(chan error, 2)

	go func() {
		grip.Info("Starting non-SSL API server")
		err := graceful.Serve(server, nonSSL, requestTimeout)
		if err != nil {
			if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
				grip.Warningf("non-SSL API server error: %+v", err)
			} else {
				err = nil
			}
		}
		grip.Info("non-SSL API server cleanly terminated")
		errChan <- err
	}()

	go func() {
		grip.Info("Starting SSL API server")
		err := graceful.Serve(server, ssl, requestTimeout)
		if err != nil {
			if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
				grip.Warningf("SSL API server error: %+v", err)
			} else {
				err = nil
			}
		}
		grip.Info("SSL API server cleanly terminated")
		errChan <- err
	}()

	exitCode := 0

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			grip.Errorf("Error returned from API server: %+v", err)
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}
