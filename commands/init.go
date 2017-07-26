package commands

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/cbednarski/mkdeb/deb"
	"github.com/facebookgo/flagenv"
	"github.com/google/subcommands"
)

type InitCmd struct {
	config string
}

func (*InitCmd) Name() string     { return "init" }
func (*InitCmd) Synopsis() string { return "create a new mkdeb config file" }
func (*InitCmd) Usage() string {
	return `init`
}

func (p *InitCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&p.config, "config", "mkdeb.json", "config file name")
}

func (p *InitCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if err := flagenv.ParseSet(flagenv.Prefix, f); err != nil {
		log.Fatal(err)
	}

	if p.config == "" {
		fmt.Println("Error: -config argument cannot be empty")
		return subcommands.ExitFailure
	}
	if err := initialize(p.config); err != nil {
		fmt.Printf("Error: %s\n", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

// initialize creates a new mkdeb config. This function is not called init()
// because that has a special meaning in Go.
func initialize(filename string) error {
	// Get abs path to PWD
	workdir, err := os.Getwd()

	workdir, err = filepath.Abs(workdir)
	if err != nil {
		return err
	}

	// Get config file name
	target := path.Join(workdir, filename)
	if deb.FileExists(target) {
		return fmt.Errorf("%s already exists in this directory", filename)
	}

	// Create config file
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create config struct
	projectName := filepath.Base(workdir)
	p := deb.DefaultPackageSpec()
	p.Package = projectName
	p.Maintainer = "Your Name <you@example.com>"
	p.Architecture = "amd64"
	p.Description = projectName + " is an awsome project for..."
	p.Homepage = "https://www.example.com/project"
	p.Files = map[string]string{projectName: "/usr/local/bin/" + projectName}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(data)
	if err != nil {
		return err
	}
	return nil
}
