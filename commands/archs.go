package commands

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/cbednarski/mkdeb/deb"
	"github.com/google/subcommands"
)

type ArchsCmd struct {
}

func (*ArchsCmd) Name() string     { return "archs" }
func (*ArchsCmd) Synopsis() string { return "list supported CPU architectures" }
func (*ArchsCmd) Usage() string {
	return `archs`
}

func (p *ArchsCmd) SetFlags(f *flag.FlagSet) {

}

func (p *ArchsCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	fmt.Printf("mkdeb supported architectures: %s\n", strings.Join(deb.SupportedArchitectures(), ", "))
	return subcommands.ExitSuccess
}
