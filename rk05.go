package pdp11

import "fmt"
import "io/ioutil"

const imglen = 2077696

const (
	RKOVR = (1 << 14)
	RKNXD = (1 << 7)
	RKNXC = (1 << 6)
	RKNXS = (1 << 5)
)

type RK05 struct {
	RKBA, RKDS, RKER, RKCS, RKWC, drive, sector, surface, cylinder, rkimg int
	running                                                               bool
	rkdisk                                                                []byte // rk0 disk image
}

func (r *RK05) rkread16(a uint18) int {
	switch a {
	case 0777400:
		return r.RKDS
	case 0777402:
		return r.RKER
	case 0777404:
		return r.RKCS | (r.RKBA&0x30000)>>12
	case 0777406:
		return r.RKWC
	case 0777410:
		return r.RKBA & 0xFFFF
	case 0777412:
		return (r.sector) | (r.surface << 4) | (r.cylinder << 5) | (r.drive << 13)
	default:
		panic("invalid read")
	}
}

func (r *RK05) rknotready() {
	r.RKDS &= ^(1 << 6)
	r.RKCS &= ^(1 << 7)
}

func (r *RK05) rkready() {
	r.RKDS |= 1 << 6
	r.RKCS |= 1 << 7
}

func (r *RK05) rkerror(code int) {
	var msg string
	r.rkready()
	r.RKER |= code
	r.RKCS |= (1 << 15) | (1 << 14)
	switch code {
	case RKOVR:
		msg = "operation overflowed the disk"
		break
	case RKNXD:
		msg = "invalid disk accessed"
		break
	case RKNXC:
		msg = "invalid cylinder accessed"
		break
	case RKNXS:
		msg = "invalid sector accessed"
		break
	}
	panic(msg)
}

func (r *RK05) Step() {
	if !r.running {
		return
	}
	var w bool
	switch (r.RKCS & 017) >> 1 {
	case 0:
		return
	case 1:
		w = true
	case 2:
		w = false
	default:
		panic(fmt.Sprintf("unimplemented RK05 operation %#o", ((r.RKCS & 017) >> 1)))
	}
	//fmt.Println("rkrwsec: RKBA:", r.RKBA, "RKWC:", r.RKWC, "cylinder:", r.cylinder, "sector:", r.sector)
	if r.drive != 0 {
		r.rkerror(RKNXD)
	}
	if r.cylinder > 0312 {
		r.rkerror(RKNXC)
	}
	if r.sector > 013 {
		r.rkerror(RKNXS)
	}
	pos := (r.cylinder*24 + r.surface*12 + r.sector) * 512
	for i := 0; i < 256 && r.RKWC != 0; i++ {
		if w {
			val := memory[r.RKBA>>1]
			r.rkdisk[pos] = byte(val & 0xFF)
			r.rkdisk[pos+1] = byte((val >> 8) & 0xFF)
		} else {
			memory[r.RKBA>>1] = uint16(r.rkdisk[pos]) | uint16(r.rkdisk[pos+1])<<8
		}
		r.RKBA += 2
		pos += 2
		r.RKWC = (r.RKWC + 1) & 0xFFFF
	}
	r.sector++
	if r.sector > 013 {
		r.sector = 0
		r.surface++
		if r.surface > 1 {
			r.surface = 0
			r.cylinder++
			if r.cylinder > 0312 {
				r.rkerror(RKOVR)
			}
		}
	}
	if r.RKWC == 0 {
		r.running = false
		r.rkready()
		if r.RKCS&(1<<6) != 0 {
			interrupt(INTRK, 5)
		}
	}
}

func (r *RK05) rkgo() {
	switch (r.RKCS & 017) >> 1 {
	case 0:
		r.rkreset()
	case 1, 2:
		r.running = true
		r.rknotready()
	default:
		panic(fmt.Sprintf("unimplemented RK05 operation %#o", ((r.RKCS & 017) >> 1)))
	}
}

func (r *RK05) rkwrite16(a uint18, v int) {
	switch a {
	//	case 0777400:
	//		break
	//	case 0777402:
	//		break
	case 0777404:
		r.RKBA = (r.RKBA & 0xFFFF) | ((v & 060) << 12)
		const BITS = 017517
		v &= BITS // writable bits
		r.RKCS &= ^BITS
		r.RKCS |= v & ^1 // don't set GO bit
		if v&1 == 1 {
			r.rkgo()
		}
	case 0777406:
		r.RKWC = v
	case 0777410:
		r.RKBA = (r.RKBA & 0x30000) | (v)
	case 0777412:
		r.drive = v >> 13
		r.cylinder = (v >> 5) & 0377
		r.surface = (v >> 4) & 1
		r.sector = v & 15
	default:
		panic("invalid write")
	}
}

func (r *RK05) rkreset() {
	r.RKDS = (1 << 11) | (1 << 7) | (1 << 6)
	r.RKER = 0
	r.RKCS = 1 << 7
	r.RKWC = 0
	r.RKBA = 0
}

func (r *RK05) rkinit() {
	var err error
	r.rkdisk, err = ioutil.ReadFile("rk0")
	if err != nil {
		panic(err)
	}
}
