package main

import (
	"go/build"
	"log"
	"os"
	"path/filepath"

	"github.com/davecheney/pdp11"
)

func stdin(c chan uint8) {
	for _, v := range []byte("unix\n") {
		c <- v
	}
	var b [1]byte
	for {
		n, err := os.Stdin.Read(b[:])
		if n == 1 {
			c <- b[0]
		}
		if err != nil {
			log.Println(pdp11.Runtime)
			log.Fatal(err)
		}
	}
}

func main() {
	pdp := pdp11.New()
	pdp.LoadMemory(pdp11.BOOTRK05)
	pdp.SetPC(002002)
	pdp.Attach(0, filepath.Join(build.Default.GOPATH, "src/github.com/davecheney/pdp11/rk0"))
	go stdin(pdp.Input)
	pdp.Run()
}
