package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/earthboundkid/versioninfo/v2"
	"github.com/spf13/pflag"
)

// AddFlag adds -v and -version flags to the FlagSet.
// If triggered, the flags print version information and call os.Exit(0).
// If FlagSet is nil, it adds the flags to flag.CommandLine.
func AddVersionFlag(f *pflag.FlagSet) {
	if f == nil {
		f = pflag.CommandLine
	}
	flag := f.VarPF(boolFunc(printVersion), "version", "v", "print version information and exit")
	flag.DefValue = "false"
	flag.NoOptDefVal = "true"
}

func printVersion(b bool) error {
	if !b {
		return nil
	}
	fmt.Println("Version:", versioninfo.Version)
	fmt.Println("Revision:", versioninfo.Revision)
	if versioninfo.Revision != "unknown" {
		fmt.Println("Committed:", versioninfo.LastCommit.Format(time.RFC1123))
		if versioninfo.DirtyBuild {
			fmt.Println("Dirty Build")
		}
	}
	os.Exit(0)
	panic("unreachable")
}

type boolFunc func(bool) error

func (f boolFunc) IsBoolFlag() bool {
	return true
}

func (f boolFunc) String() string {
	return ""
}

func (f boolFunc) Type() string {
	return "bool"
}

func (f boolFunc) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	return f(b)
}
