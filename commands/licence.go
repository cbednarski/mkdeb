package commands

import (
	"context"
	"flag"
	"fmt"

	"github.com/google/subcommands"
)

type LicenceCmd struct {
}

func (*LicenceCmd) Name() string     { return "licence" }
func (*LicenceCmd) Synopsis() string { return "show licence information" }
func (*LicenceCmd) Usage() string {
	return licenceText
}

func (p *LicenceCmd) SetFlags(f *flag.FlagSet) {}

func (p *LicenceCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {

	fmt.Println(licenceText)
	return subcommands.ExitSuccess
}

const licenceText = `Copyright 2016 Chris Bednarski <banzaimonkey@gmail.com>, and others

  Portions of mkdeb are licensed under the MIT, BSD and Go Licenses. Please
  refer to the project source for full license text and details.

  https://github.com/cbednarski/mkdeb`
