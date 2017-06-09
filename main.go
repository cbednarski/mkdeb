package main

import (
	"context"
	"flag"
	"os"

	"github.com/cbednarski/mkdeb/commands"
	"github.com/facebookgo/flagenv"
	"github.com/google/subcommands"
)

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&commands.BuildCmd{}, "")
	subcommands.Register(&commands.ArchsCmd{}, "")
	subcommands.Register(&commands.InitCmd{}, "")
	subcommands.Register(&commands.PackagingCmd{}, "")
	subcommands.Register(&commands.LicenceCmd{}, "")
	subcommands.Register(&commands.ValidateCmd{}, "")
	flagenv.Prefix="deb_"
	flagenv.Parse()
	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
