package hostutil

import (
	"bytes"
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// sshTimeout is the timeout for SSH commands.
	sshTimeout = 2 * time.Minute
)

// RunSSHCommand runs an SSH command on a remote host.
func RunSSHCommand(ctx context.Context, cmd string, sshOptions []string, host host.Host) (string, error) {
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(host.Host)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing ssh info %v", host.Host)
	}

	output := newCappedOutputLog()
	opts := subprocess.OutputOptions{Output: output, SendErrorToOutput: true}
	proc := subprocess.NewRemoteCommand(
		cmd,
		hostInfo.Hostname,
		host.User,
		nil,   // env
		false, // background
		append([]string{"-p", hostInfo.Port, "-t", "-t"}, sshOptions...),
		false, // loggingDisabled
	)

	if err = proc.SetOutput(opts); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"function":  "RunSSHCommand",
			"operation": "setting up command output",
			"distro":    host.Distro.Id,
			"host":      host.Id,
			"output":    output,
			"cause":     "programmer error",
		}))

		return "", errors.Wrap(err, "problem setting up command output")
	}

	grip.Info(message.Fields{
		"command": proc,
		"host_id": host.Id,
		"message": "running command over ssh",
	})

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, sshTimeout)
	defer cancel()

	err = proc.Run(ctx)
	grip.Notice(proc.Stop())
	return output.String(), errors.Wrap(err, "error running shell cmd")
}

func newCappedOutputLog() *util.CappedWriter {
	// store up to 1MB of streamed command output to print if a command fails
	return &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}
}
