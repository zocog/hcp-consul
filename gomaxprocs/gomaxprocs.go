package main

import (
    "fmt"
    "runtime"
)

func main() {
    fmt.Printf("GOMAXPROCS is %d\n", runtime.GOMAXPROCS(0))
}

