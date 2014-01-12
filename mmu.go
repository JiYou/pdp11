package pdp11

import "fmt"

type mmu struct {
	SR0, SR2 uint16
	cpu      *KB11
	pages    [16]page
}

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

func (m *mmu) mmuread16(a uint18) uint16 {
	i := ((a & 017) >> 1)
	if (a >= 0772300) && (a < 0772320) {
		return m.pages[i].pdr
	}
	if (a >= 0772340) && (a < 0772360) {
		return m.pages[i].par
	}
	if (a >= 0777600) && (a < 0777620) {
		return m.pages[i+8].pdr
	}
	if (a >= 0777640) && (a < 0777660) {
		return m.pages[i+8].par
	}
	panic(trap{INTBUS, fmt.Sprintf("invalid read from %06o", a)})
}

func (m *mmu) mmuwrite16(a uint18, v uint16) {
	i := ((a & 017) >> 1)
	if (a >= 0772300) && (a < 0772320) {
		m.pages[i] = createpage(m.pages[i].par, v)
		return
	}
	if (a >= 0772340) && (a < 0772360) {
		m.pages[i] = createpage(v, m.pages[i].pdr)
		return
	}
	if (a >= 0777600) && (a < 0777620) {
		m.pages[i+8] = createpage(m.pages[i+8].par, v)
		return
	}
	if (a >= 0777640) && (a < 0777660) {
		m.pages[i+8] = createpage(v, m.pages[i+8].pdr)
		return
	}
	panic(trap{INTBUS, fmt.Sprintf("write to invalid address %06o", a)})
}

func (m *mmu) decode(a uint16, w, user bool) uint18 {
	var p page
	if !(m.SR0&1 == 1) {
		if a := uint18(a); a >= 0170000 {
			a += 0600000
			return a
		} else {
			return a
		}
	}
	if user {
		p = m.pages[(a>>13)+8]
	} else {
		p = m.pages[(a >> 13)]
	}
	if w && !p.write {
		m.SR0 = (1 << 13) | 1
		m.SR0 |= a >> 12 & ^uint16(1)
		if user {
			m.SR0 |= (1 << 5) | (1 << 6)
		}
		m.SR2 = m.cpu.PC
		panic(trap{INTFAULT, fmt.Sprintf("write to read-only page %06o", a)})
	}
	if !p.read {
		m.SR0 = (1 << 15) | 1
		m.SR0 |= (a >> 12) & ^uint16(1)
		if user {
			m.SR0 |= (1 << 5) | (1 << 6)
		}
		m.SR2 = m.cpu.PC
		panic(trap{INTFAULT, fmt.Sprintf("read from no-access page %06o", a)})
	}
	block := a >> 6 & 0177
	disp := uint18(a & 077)
	if p.ed && block < p.len || !p.ed && block > p.len {
		//if(p.ed ? (block < p.len) : (block > p.len)) {
		m.SR0 = (1 << 14) | 1
		m.SR0 |= (a >> 12) & ^uint16(1)
		if user {
			m.SR0 |= (1 << 5) | (1 << 6)
		}
		m.SR2 = m.cpu.PC
		panic(trap{INTFAULT, fmt.Sprintf("page length exceeded, address %06o (block %03o) is beyond %03o", a, block, p.len)})
	}
	if w {
		p.pdr |= 1 << 6
	}
	return (uint18(block+p.addr) << 6) + disp
}
