package main

import (
	"log"
	"os"
	"pdp11"
)

func stdin(c chan uint8) {
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
	pdp11.RKINIT()
	cpu := pdp11.New()
	go stdin(cpu.Input)
	for {
		cpu.Step()
	}
}
