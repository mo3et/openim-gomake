//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mo3et/openim-gomake/mageutil"
)

var Default = Build

func parseBinFlag(args []string) string {
	for i, arg := range args {
		if arg == "-bin" || arg == "--bin" {
			if i+1 < len(args) {
				return args[i+1]
			}
		} else if strings.HasPrefix(arg, "-bin=") || strings.HasPrefix(arg, "--bin=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}
	return ""
}

func Build() {
	var binaryList []string

	binFlag := parseBinFlag(os.Args)

	if binFlag != "" {
		binaryList = strings.Split(binFlag, ",")
	}
	fmt.Println("binaryList: ", binaryList)
	mageutil.Build(binaryList)
}

func Start() {
	mageutil.InitForSSC()
	err := setMaxOpenFiles()
	if err != nil {
		mageutil.PrintRed("setMaxOpenFiles failed " + err.Error())
		os.Exit(1)
	}
	mageutil.StartToolsAndServices()
}

func Stop() {
	mageutil.StopAndCheckBinaries()
}

func Check() {
	mageutil.CheckAndReportBinariesStatus()
}

func Protocol() {
	mageutil.Protocol()
}
