package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(runCLI(os.Args[1:], os.Stderr, productionDependencies())) }

func runCLI(args []string, out io.Writer, d dependencies) int {
	flags := flag.NewFlagSet("dyndns-updater", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("config", "/etc/dyndns-updater.yaml", "YAML configuration file")
	flags.Usage = func() {
		fmt.Fprintln(out, "dyndns-updater: one-shot IPv4 dynamic DNS client\nUsage: dyndns-updater [--config FILE]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(out, "unexpected arguments")
		return 1
	}
	c, err := loadConfig(*path)
	if err == nil {
		err = update(c, d, out)
	}
	if err != nil {
		fmt.Fprintln(out, "dyndns-updater:", err)
		return 1
	}
	return 0
}
