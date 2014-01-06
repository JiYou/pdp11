package pdp11

import "fmt"
import "io/ioutil"

var RKBA int
var RKDS, RKER, RKCS, RKWC, drive, sector, surface, cylinder, rkimg uint16

const imglen = 2077696;

const (
    RKOVR = (1<<14)
    RKNXD = (1<<7)
    RKNXC = (1<<6)
    RKNXS = (1<<5)
   )

func rkread16(a uint32) uint16 {
	switch(a) {
	case 0777400: return RKDS;
	case 0777402: return RKER;
	case 0777404: return RKCS | uint16((RKBA & 0x30000) >> 12);
	case 0777406: return RKWC;
	case 0777410: return uint16(RKBA & 0xFFFF)
	case 0777412: return (sector) | (surface << 4) | (cylinder << 5) | (drive << 13);
	default:
		panic("invalid read");
	}
}

func rknotready() {
	RKDS &= ^uint16(1<<6);
	RKCS &= ^uint16(1<<7);
}

func rkready() {
	RKDS |= 1<<6;
	RKCS |= 1<<7;
}

func rkerror(code uint16) {	
	var msg string
	rkready();
	RKER |= code;
	RKCS |= (1<<15) | (1<<14);
	switch(code) {
	case RKOVR: msg = "operation overflowed the disk"; break;
	case RKNXD: msg = "invalid disk accessed"; break;
	case RKNXC: msg = "invalid cylinder accessed"; break;
	case RKNXS: msg = "invalid sector accessed"; break;
	}
	panic(msg);
}

func rkrwsec(t bool) {
	if(drive != 0) { rkerror(RKNXD); }
	if(cylinder > 0312) { rkerror(RKNXC) }
	if(sector > 013) { rkerror(RKNXS) }
	pos := (cylinder * 24 + surface * 12 + sector) * 512;
	for i:=0;i<256 && RKWC != 0;i++ {
		if(t) {
			val := memory[RKBA >> 1];
			rkdisk[pos] = byte(val & 0xFF)
			rkdisk[pos+1] = byte((val >> 8) & 0xFF)
		} else {
			memory[RKBA >> 1] = uint16(rkdisk[pos]) | uint16((rkdisk[pos+1] << 8))
		}
		RKBA += 2;
		pos += 2;
		RKWC = (RKWC + 1) & 0xFFFF;
	}
	sector++;
	if(sector > 013) {
		sector = 0;
		surface++;
		if(surface > 1) {
			surface = 0;
			cylinder++;
			if(cylinder > 0312) {
				rkerror(RKOVR);
			}
		}
	}
	if(RKWC!= 0) {
		rkrwsec(t)
	} else {
		rkready();
		if(RKCS & (1<<6) != 0) { interrupt(INTRK, 5) }
	}
}

func rkgo() {
	switch((RKCS & 017) >> 1) {
	case 0: rkreset(); break;
	case 1: rknotready(); rkrwsec(true); break;
	case 2: rknotready(); rkrwsec(false); break;
	default: panic(fmt.Sprintf("unimplemented RK05 operation %#o" , ((RKCS & 017) >> 1)));
	}
}

func rkwrite16(a uint32,v uint16) {
	switch(a) {
	case 0777400: break;
	case 0777402: break;
	case 0777404:
		RKBA = int(uint16(RKBA & 0xFFFF) | ((v & 060) << 12))
		v &= 017517; // writable bits
		RKCS &= uint16(^uint16(017517))
		const MASK uint16 = 1
		RKCS |= v & ^MASK; // don't set GO bit
		if(v & 1 == 1) { rkgo() }
		break;
	case 0777406: RKWC = v; break;
	case 0777410: RKBA = (RKBA & 0x30000) | int(v); break;
	case 0777412:
		drive = v >> 13;
		cylinder = (v >> 5) & 0377;
		surface = (v >> 4) & 1;
		sector = v & 15;
		break;
	default:
		panic("invalid write");
	}
}

func rkreset() {
	RKDS = (1 << 11) | (1 << 7) | (1 << 6);
	RKER = 0;
	RKCS = 1 << 7;
	RKWC = 0;
	RKBA = 0;
	// RKDB = 0;
}

var rkdisk []byte

func rkinit() {
	var err error
	rkdisk, err = ioutil.ReadFile("rk0")
	if err != nil { panic(err) }
}
