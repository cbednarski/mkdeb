package commands

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cbednarski/mkdeb/deb"
	"github.com/facebookgo/flagenv"
	"github.com/google/subcommands"
)

type ValidateCmd struct {
	config string
}

func (*ValidateCmd) Name() string     { return "validate" }
func (*ValidateCmd) Synopsis() string { return "validate config file" }
func (*ValidateCmd) Usage() string {
	return `validate [-config] mkdeb.json:
`
}

func (p *ValidateCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&p.config, "config", "", "config file")
}

func (p *ValidateCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if err := flagenv.ParseSet(flagenv.Prefix, f); err != nil {
		log.Fatal(err)
	}

	var config string
	if f.NArg() > 0 {
		config = f.Arg(0)
	}
	if p.config != "" {
		if config != "" {
			fmt.Println("error: only use one of positional or -config argument for config file")
			return subcommands.ExitFailure
		}
		config = p.config
	}
	if config == "" {
		fmt.Println("Error: config file not specified")
		return subcommands.ExitFailure
	}

	if err := validate(config); err != nil {
		fmt.Printf("Error: %s\n", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func validate(config string) error {
	// Change to config path
	back, err := os.Getwd()
	if err != nil {
		return err
	}
	workdir, filename := getAbsPaths(config)
	err = os.Chdir(workdir)
	if err != nil {
		return err
	}
	defer os.Chdir(back)
	fmt.Println(workdir, filename)
	// Validate
	p, err := deb.NewPackageSpecFromFile(filename)
	if err != nil {
		return err
	}
	err = p.Validate(false)
	if err != nil {
		return err
	}
	return nil
}
