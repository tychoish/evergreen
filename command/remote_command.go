package command

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/tychoish/grip"
)

type RemoteCommand struct {
	Id        string
	CmdString string

	Stdout io.Writer
	Stderr io.Writer

	// info necessary for sshing into the remote host
	RemoteHostName string
	User           string
	Options        []string
	Background     bool

	// optional flag for hiding sensitive commands from log output
	LoggingDisabled bool

	// set after the command is started
	Cmd *exec.Cmd
}

func (rc *RemoteCommand) Run() error {
	grip.Debugf("RemoteCommand(%s) beginning Run()", rc.Id)
	err := rc.Start()
	if err != nil {
		return err
	}
	if rc.Cmd != nil && rc.Cmd.Process != nil {
		grip.Debugf("RemoteCommand(%s) started process %d", rc.Id, rc.Cmd.Process.Pid)
	} else {
		grip.Debugf("RemoteCommand(%s) has nil Cmd or Cmd.Process in Run()", rc.Id)
	}
	return rc.Cmd.Wait()
}

func (rc *RemoteCommand) Wait() error {
	return rc.Cmd.Wait()
}

func (rc *RemoteCommand) Start() error {

	// build the remote connection, in user@host format
	remote := rc.RemoteHostName
	if rc.User != "" {
		remote = fmt.Sprintf("%v@%v", rc.User, remote)
	}

	// build the command
	cmdArray := append(rc.Options, remote)

	// set to the background, if necessary
	cmdString := rc.CmdString
	if rc.Background {
		cmdString = fmt.Sprintf("nohup %v > /tmp/start 2>&1 &", cmdString)
	}
	cmdArray = append(cmdArray, cmdString)

	grip.WarningWhenf(!rc.LoggingDisabled, "Remote command executing: '%#v'",
		strings.Join(cmdArray, " "))

	// set up execution
	cmd := exec.Command("ssh", cmdArray...)
	cmd.Stdout = rc.Stdout
	cmd.Stderr = rc.Stderr

	// cache the command running
	rc.Cmd = cmd
	return cmd.Start()
}

func (rc *RemoteCommand) Stop() error {
	if rc.Cmd != nil && rc.Cmd.Process != nil {
		grip.Debugf("RemoteCommand(%s) killing process %d", rc.Id, rc.Cmd.Process.Pid)
		return rc.Cmd.Process.Kill()
	}
	grip.Warningf("RemoteCommand(%s) Trying to stop command but Cmd / Process was nil", rc.Id)
	return nil
}
