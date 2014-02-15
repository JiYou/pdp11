package main

import (
	"go/build"
	"log"
	"os"
	"path/filepath"

	"github.com/davecheney/pdp11"
)

func stdin(c chan uint8) {
	c <- 'u'
	c <- 'n'
	c <- 'i'
	c <- 'x'
	c <- '\n'
	var b [1]byte
	for {
		n, err := os.Stdin.Read(b[:])
		if n == 1 {
			c <- b[0]
		}
		if err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	pdp := pdp11.New()
	const LOADADDR = 01000
	pdp.LoadBootrom(LOADADDR, pdp11.BOOTRK05)
	pdp.SetPC(LOADADDR*2 + 2)
	pdp.Attach(0, filepath.Join(build.Default.GOPATH, "src/github.com/davecheney/pdp11/rk0"))
	go stdin(pdp.Input)
	pdp.Run()
}
