package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	var (
		race      bool
		arch      string
		system    string
		directory string
		source    string
		ldFlags   string
		buildName string
	)

	flag.BoolVar(&race, "race", false, "build with the race detector")
	flag.StringVar(&arch, "goarch", runtime.GOARCH, "target architecture (GOARCH)")
	flag.StringVar(&system, "goos", runtime.GOOS, "target system (GOOS)")
	flag.StringVar(&directory, "directory", "", "output directory")
	flag.StringVar(&source, "source", "", "path to source file")
	flag.StringVar(&ldFlags, "ldflags", "", "specify any ldflags to pass to go build")
	flag.StringVar(&buildName, "buildName", "", "use GOOS_ARCH to specify target platform")
	flag.Parse()

	if buildName != "" {
		parts := strings.Split(buildName, "_")

		system = parts[0]
		arch = parts[1]
	} else {
		buildName = fmt.Sprintf("%s_%s", system, arch)
	}

	output := filepath.Join(directory, buildName, "main")
	cmd := exec.Command("go", "build")

	if race {
		cmd.Args = append(cmd.Args, "-race")
	}

	cmd.Args = append(cmd.Args, "-o", output)
	if ldFlags != "" {
		cmd.Args = append(cmd.Args, "-ldflags=\""+ldFlags+"\"")
	}
	cmd.Args = append(cmd.Args, source)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		"GOPATH=" + os.Getenv("GOPATH"),
		"GOOS=" + system,
		"GOARCH=" + arch,
	}

	fmt.Println(strings.Join(cmd.Env, " "), strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		fmt.Printf("problem building %s: %v\n", output, err)
		os.Exit(1)
	}
}
