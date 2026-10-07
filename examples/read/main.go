// The example reads one selected profile and performs one products request.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	sber "github.com/vasyza/sber-go"
)

func main() {
	profile := flag.String("profile", "", "explicit private session path")
	flag.Parse()
	if *profile == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/read --profile PATH")
		os.Exit(2)
	}
	parent, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	if err := read(ctx, *profile); err != nil {
		fmt.Fprintln(os.Stderr, "cannot read the selected session or complete the products request")
		os.Exit(1)
	}
}

func read(ctx context.Context, profile string) (err error) {
	client, err := sber.NewSberClientFromSessionFile(profile, sber.ClientOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := client.Close(); err == nil {
			err = closeErr
		}
	}()
	products, err := client.Products().Get(ctx, false)
	if err != nil {
		return err
	}
	raw, err := sber.ExportJSON(products)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	n, err := os.Stdout.Write(raw)
	if err == nil && n != len(raw) {
		return fmt.Errorf("output failed")
	}
	return err
}
