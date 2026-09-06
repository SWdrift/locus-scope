package main

import (
	"os"

	"locus-scope/internal/pkgcli"
)

func main() {
	os.Exit(pkgcli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
