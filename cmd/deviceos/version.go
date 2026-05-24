package main

import (
	"fmt"

	"github.com/lohtbrok/deviceos/internal/version"
)

func cmdVersion() {
	bi := version.ReadBuildInfo()
	fmt.Println(version.Banner())
	fmt.Println()
	fmt.Println(bi.String())
}
