package main

import (
	"fmt"
	"os"

	"github.com/vasyza/sber-go/internal/rentalcli"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: rental-check < explicit-ledger.json")
		os.Exit(2)
	}
	os.Exit(rentalcli.Run(os.Stdin, os.Stdout, os.Stderr))
}
