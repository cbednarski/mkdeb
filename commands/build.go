package commands

import (
	"context"
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

// BuildCmd .
type BuildCmd struct {
	version string
	target  string
	config  string // alternative to positional argument
}

func (*BuildCmd) Name() string     { return "build" }
func (*BuildCmd) Synopsis() string { return "build a package based on the specified config file" }
func (*BuildCmd) Usage() string {
	return `build -version=1.2.0 [-config] config.json
By default the build artifact

The build command will change to the directory where the config file is
located, so paths should always be specified relative to the config file.

`
}

func (b *BuildCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&b.version, "version", "1.0", "Package version")
	f.StringVar(&b.target, "target", "", "Target folder with generated filename")
	f.StringVar(&b.config, "config", "", "Config file (alternative to positional argument)")
}

func (b *BuildCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if err := flagenv.ParseSet(flagenv.Prefix, f); err != nil {
		log.Fatal(err)
	}

	var config string
	if f.NArg() > 0 {
		config = f.Arg(0)
	}
	if b.config != "" {
		if config != "" {
			fmt.Println("error: only use one of positional or -config argument for config file")
			return subcommands.ExitFailure
		}
		config = b.config
	}
	if config == "" {
		fmt.Println("Error: config file not specified")
		return subcommands.ExitFailure
	}

	if err := build(config, b.version, b.target); err != nil {
		fmt.Printf("Error: %s\n", err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

// getAbsPaths takes a relative path to a file and returns both the containing
// directory and the absolute path to the file.
//
// Example: cat -> /bin /bin/cat
func getAbsPaths(filename string) (string, string) {
	path, err := filepath.Abs(filename)
	if err != nil {
		fmt.Printf("Can't find %q", filename)
		os.Exit(1)
	}
	dir, _ := filepath.Split(path)
	return dir, path
}

func build(config, version, target string) error {
	// Change to config path
	back, err := os.Getwd()
	if err != nil {
		return err
	}

	// Get the working directory to cd into and the absolute path to the file
	workdir, abspath := getAbsPaths(config)
	if err := os.Chdir(workdir); err != nil {
		return err
	}
	defer os.Chdir(back)

	p, err := deb.NewPackageSpecFromFile(abspath)
	if err != nil {
		return err
	}
	// Set version
	p.Version = version

	// Set target filename
	if target == "" {
		target = workdir
	} else {
		info, err := os.Stat(target)
		if !(err == nil && info.IsDir()) {
			return fmt.Errorf("%q is not a directory", target)

		}
	}

	// Validate
	if err := p.Validate(true); err != nil {
		return err
	}

	// Build
	if err := p.Build(target); err != nil {
		return err
	}

	fmt.Printf("Built package %s\n", path.Join(target, p.Filename()))
	return nil
}
