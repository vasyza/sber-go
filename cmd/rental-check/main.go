package main

import (
	"context"
	"os"

	"github.com/vasyza/sber-go/internal/rentalcli"
)

func main() {
	os.Exit(rentalcli.RunCommand(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
