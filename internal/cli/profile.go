package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errDefaultProfile = errors.New("default profile path is not valid")

func defaultProfilePath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(directory) || strings.IndexByte(directory, 0) >= 0 {
		return "", errDefaultProfile
	}
	return filepath.Join(directory, "sber-sdk", "profile.json"), nil
}

func selectProfile(args *commandArguments, resolve func() (string, error)) error {
	if args.profile != "" {
		return nil
	}
	if resolve == nil {
		resolve = defaultProfilePath
	}
	profile, err := resolve()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(profile) || filepath.Clean(profile) != profile || strings.IndexByte(profile, 0) >= 0 {
		return errDefaultProfile
	}
	args.profile, args.defaultProfile = profile, true
	return nil
}
