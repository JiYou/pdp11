package pdp11

import "os"

//import "fmt"

var TKS, TKB, TPS, TPB int

func clearterminal() {
	TKS = 0
	TPS = 1 << 7
	TKB = 0
	TPB = 0
}

func writeterminal(c int) {
	os.Stdout.Write([]byte{byte(c)})
}

func addchar(c int) {
	switch c {
	case 42:
		TKB = 4
	case 19:
		TKB = 034
	case 46:
		TKB = 127
	default:
		TKB = c
	}
	TKS |= 0x80
	if TKS&(1<<6) != 0 {
		interrupt(INTTTYIN, 4)
	}
}

var input []int

func getchar() int {
	if TKS&0x80 == 0x80 {
		TKS &= 0xff7e
		return TKB
	}
	return 0
}

var count uint8

func StepConsole(k *KB11) {
	if waiting {
		// console not busy
		if c, ok := <-k.Input; ok {
			addchar(int(c))
		}
	}
	if count++; count != 0 {
		return
	}
	if TPS&0x80 == 0 {
		TPS |= 0x80
		writeterminal(TPB & 0x7f)
		if TPS&(1<<6) != 0 {
			interrupt(INTTTYOUT, 4)
		}
	}
}

func consread16(a int) int {
	//fmt.Printf("consread16: %o\n", a)
	switch a {
	case 0777560:
		if len(input) > 0 {
			addchar(input[0])
			input = input[1:]
		}
		return TKS
	case 0777562:
		return getchar()
	case 0777564:
		return TPS
	case 0777566:
		return 0
	}
	panic("read from invalid address " + ostr(a, 6))
}

func conswrite16(a, v int) {
	//fmt.Printf("conswrite16: %o, %o\n", a,v )
	switch a {
	case 0777560:
		if v&(1<<6) != 0 {
			TKS |= 1 << 6
		} else {
			TKS &= ^(1 << 6)
		}
	case 0777564:
		if v&(1<<6) != 0 {
			TPS |= 1 << 6
		} else {
			TPS &= ^(1 << 6)
		}
	case 0777566:
		TPB = v & 0xff
		TPS &= 0xff7f
	default:
		panic("write to invalid address " + ostr(a, 6))
	}
}
