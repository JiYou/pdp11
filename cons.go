package pdp11

import (
	"os"
)

type Console struct {
	TKS, TKB, TPS, TPB int

	Input chan uint8
	count uint8 // step delay
	ready bool
}

func (c *Console) clearterminal() {
	c.TKS = 0
	c.TPS = 1 << 7
	c.TKB = 0
	c.TPB = 0
	c.ready = true
}

func (c *Console) writeterminal(char int) {
	var outb [1]byte
	switch char {
	case 13:
		// skip
	default:
		outb[0] = byte(char)
		os.Stdout.Write(outb[:])
	}
}

func (c *Console) addchar(char int) {
	switch char {
	case 42:
		c.TKB = 4
	case 19:
		c.TKB = 034
	case 46:
		c.TKB = 127
	default:
		c.TKB = char
	}
	c.TKS |= 0x80
	c.ready = false
	if c.TKS&(1<<6) != 0 {
		interrupt(INTTTYIN, 4)
	}
}

func (c *Console) getchar() int {
	if c.TKS&0x80 == 0x80 {
		c.TKS &= 0xff7e
		c.ready = true
		return c.TKB
	}
	return 0
}

func (c *Console) Step(k *KB11) {
	if c.TKS & 1 << 6 == 0 {
		select {
		case v, ok := <-c.Input:
			if ok {
				c.addchar(int(v))
			}
		default:
		}
	}
	c.count++
	if c.count%32 != 0 {
		return
	}
	if c.TPS&0x80 == 0 {
		c.writeterminal(c.TPB & 0x7f)
		c.TPS |= 0x80
		if c.TPS&(1<<6) != 0 {
			interrupt(INTTTYOUT, 4)
		}
	}
}

func (c *Console) consread16(a int) int {
	switch a {
	case 0777560:
		return c.TKS
	case 0777562:
		return c.getchar()
	case 0777564:
		return c.TPS
	case 0777566:
		return 0
	default:
		panic("read from invalid address " + ostr(a, 6))
	}
}

func (c *Console) conswrite16(a, v int) {
	switch a {
	case 0777560:
		if v&(1<<6) != 0 {
			c.TKS |= 1 << 6
		} else {
			c.TKS &= ^(1 << 6)
		}
	case 0777564:
		if v&(1<<6) != 0 {
			c.TPS |= 1 << 6
		} else {
			c.TPS &= ^(1 << 6)
		}
	case 0777566:
		c.TPB = v & 0xff
		c.TPS &= 0xff7f
	default:
		panic("write to invalid address " + ostr(a, 6))
	}
}
