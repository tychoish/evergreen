package command

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLocalCommands(t *testing.T) {

	Convey("When running local commands", t, func() {

		Convey("the preparation step should replace expansions and forward"+
			" slashes in the command string", func() {

			command := &LocalCommand{
				CmdString: "one ${two} \\three${four|five}",
			}

			expansions := NewExpansions(map[string]string{
				"two": "TWO",
				"six": "SIX",
			})

			// run the preparation stage, and make sure the replacements are
			// correctly made
			So(command.PrepToRun(expansions), ShouldBeNil)
			So(command.CmdString, ShouldEqual, "one TWO \\threefive")

		})

		Convey("the specified environment should be used", func() {

			stdout := &CacheLastWritten{}

			command := &LocalCommand{
				CmdString: "echo $local_command_test",
				Stdout:    stdout,
				Stderr:    ioutil.Discard,
			}

			// get the current env
			command.Environment = os.Environ()

			// run the command - the environment variable should be empty
			So(command.Run(), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, "\n")

			// add the environment variable to the env
			command.Environment = append(command.Environment,
				"local_command_test=hello")

			// run the command again - the environment variable should be set
			// correctly
			So(command.Run(), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, "hello\n")

		})

		Convey("the specified working directory should be used", func() {

			stdout := &CacheLastWritten{}

			workingDir, err := filepath.Abs(evergreen.FindEvergreenHome())
			So(err, ShouldBeNil)

			command := &LocalCommand{
				CmdString:        "pwd",
				Stdout:           stdout,
				Stderr:           ioutil.Discard,
				WorkingDirectory: workingDir,
			}

			// run the command - the working directory should be as specified
			So(command.Run(), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, workingDir+"\n")

		})

		Convey("the specified shell should be used", func() {
			for _, sh := range []string{"bash", "sh", "/bin/bash", "/bin/sh"} {
				stdout := &CacheLastWritten{}
				command := &LocalCommand{
					Shell:     sh,
					CmdString: "echo $0",
					Stdout:    stdout,
					Stderr:    ioutil.Discard,
				}

				So(command.Run(), ShouldBeNil)
				So(string(stdout.LastWritten), ShouldEqual, sh+"\n")
			}
		})

		Convey("if not specified, sh should be used", func() {
			stdout := &CacheLastWritten{}
			command := &LocalCommand{
				CmdString: "echo $0",
				Stdout:    stdout,
				Stderr:    ioutil.Discard,
			}

			So(command.Run(), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, "sh\n")
		})

		Convey("when specified, local command can also use python", func() {
			stdout := &CacheLastWritten{}
			command := &LocalCommand{
				Shell:     "python",
				CmdString: "print('hello world')",
				Stdout:    stdout,
				Stderr:    ioutil.Discard,
			}

			So(command.Run(), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, "hello world\n")
		})

	})
}

func TestLocalCommandGroups(t *testing.T) {

	Convey("With a group of local commands", t, func() {

		Convey("the global preparation step should invoke all of the prep"+
			" steps for the group", func() {

			// the three commands for the group, whose preparation steps will
			// yield different results for expanding the command string
			firstCommand := &LocalCommand{
				CmdString: "one\\ ${two} \\three",
			}
			secondCommand := &LocalCommand{
				CmdString: "${four|five}",
			}
			thirdCommand := &LocalCommand{
				CmdString: "six seven",
			}

			expansions := NewExpansions(map[string]string{
				"two": "TWO",
				"six": "SIX",
			})

			cmdGroup := &LocalCommandGroup{
				Commands: []*LocalCommand{firstCommand, secondCommand,
					thirdCommand},
				Expansions: expansions,
			}

			// run the preparation step for the command group, make sure it is
			// run for each command individually
			So(cmdGroup.PrepToRun(), ShouldBeNil)
			So(firstCommand.CmdString, ShouldEqual, "one\\ TWO \\three")
			So(secondCommand.CmdString, ShouldEqual, "five")
			So(thirdCommand.CmdString, ShouldEqual, "six seven")

		})

		Convey("The global preparation step should fail if any of the"+
			" individual group members' prep steps fail", func() {

			// the three commands for the group. only the second will error
			firstCommand := &LocalCommand{
				CmdString: "one\\ ${two} \\three",
			}
			secondCommand := &LocalCommand{
				CmdString: "${four|five}${",
			}
			thirdCommand := &LocalCommand{
				CmdString: "six seven",
			}

			expansions := NewExpansions(map[string]string{
				"two": "TWO",
				"six": "SIX",
			})

			cmdGroup := &LocalCommandGroup{
				Commands: []*LocalCommand{firstCommand, secondCommand,
					thirdCommand},
				Expansions: expansions,
			}

			// the preparation step should fail
			So(cmdGroup.PrepToRun(), ShouldNotBeNil)

		})

	})

}

func TestLocalScript(t *testing.T) {
	Convey("When running local commands in script mode", t, func() {

		Convey("A multi-line script should run all lines", func() {

			stdout := &CacheLastWritten{}

			workingDir, err := filepath.Abs(evergreen.FindEvergreenHome())
			So(err, ShouldBeNil)

			command := &LocalCommand{
				CmdString:        "set -v\necho 'hi'\necho 'foo'\necho `pwd`",
				ScriptMode:       true,
				Stdout:           stdout,
				Stderr:           ioutil.Discard,
				WorkingDirectory: workingDir,
			}

			// run the command - the working directory should be as specified
			So(command.Run(), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, workingDir+"\n")
		})

	})
}
