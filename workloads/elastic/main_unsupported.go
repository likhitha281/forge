//go:build !linux

package main

import "fmt"

func main() {
	fmt.Println(
		"elastic workload is supported on Linux only",
	)
}
