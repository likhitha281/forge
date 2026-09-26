//go:build !linux

package main

import "fmt"

func main() {
	fmt.Println(
		"checkpointable workload is supported only on Linux",
	)
}
