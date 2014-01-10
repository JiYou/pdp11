package pdp11

var pages [16]page

type page struct {
	par, pdr        uint16
	addr, len       uint16
	read, write, ed bool
}

func createpage(par, pdr uint16) page {
	return page{
		par:   par,
		pdr:   pdr,
		addr:  par & 07777,
		len:   pdr >> 8 & 0x7F,
		read:  (pdr & 2) == 2,
		write: (pdr & 6) == 6,
		ed:    (pdr & 8) == 8,
	}
}

func mmuread16(a int) uint16 {
	i := ((a & 017) >> 1)
	if (a >= 0772300) && (a < 0772320) {
		return pages[i].pdr
	}
	if (a >= 0772340) && (a < 0772360) {
		return pages[i].par
	}
	if (a >= 0777600) && (a < 0777620) {
		return pages[i+8].pdr
	}
	if (a >= 0777640) && (a < 0777660) {
		return pages[i+8].par
	}
	panic(trap{INTBUS, "invalid read from " + ostr(a, 6)})
}

func mmuwrite16(a int, v uint16) {
	i := ((a & 017) >> 1)
	if (a >= 0772300) && (a < 0772320) {
		pages[i] = createpage(pages[i].par, v)
		return
	}
	if (a >= 0772340) && (a < 0772360) {
		pages[i] = createpage(v, pages[i].pdr)
		return
	}
	if (a >= 0777600) && (a < 0777620) {
		pages[i+8] = createpage(pages[i+8].par, v)
		return
	}
	if (a >= 0777640) && (a < 0777660) {
		pages[i+8] = createpage(v, pages[i+8].pdr)
		return
	}
	panic(trap{INTBUS, "write to invalid address " + ostr(a, 6)})
}
