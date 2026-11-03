package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "dyndns-updater: dynamic DNS client (under development)")
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: dyndns-updater --help")
	}
	flag.Parse()
	fmt.Fprintln(os.Stderr, "dyndns-updater: updates are not implemented yet")
	os.Exit(1)
}
