package pdp11

import "testing"
import "time"

func TestXOR(t *testing.T) {
	for _, tt := range []struct {
		x, y, z bool
	}{
		{false, false, false},
		{true, false, true},
		{false, true, true},
		{true, true, false},
	} {
		got := xor(tt.x, tt.y)
		if got != tt.z {
			t.Errorf("xor(%v, %v) = %v; want %v", tt.x, tt.y, got, tt.z)
		}
	}
}

const N = 4 * 1000 * 1000

var pdpTests = []struct {
	input  string
	cycles int
}{
	//	{"STTY -LCASE\n", N},
	//	{"\ndate\n", N},    // processor loops
	//	{"\nls /bin\n", N}, // read from odd address
	//	{"\nwho\n", N},     // read from no-access page 01002
	//	{"\ncat /etc/passwd\n", N},
	{`

chdir /usr/sys/conf
cc mkconf.c
mv a.out mkconf
./mkconf
`, 3 * N},
	{`
ed TEST.C
a
main() {
printf("Hello, world!\n");
}
.
w
q
cc test.c
./a.out
`, 4 * N},
}

func TestPDP(t *testing.T) {
	for _, tt := range pdpTests {
		pdp := New()
		pdp.LoadBootrom(01000, BOOTRK05) // word addressed
		pdp.SetPC(02002)                 // byte addressed
		go func() {
			c := pdp.cpu.Input
			c <- 'u'
			c <- 'n'
			c <- 'i'
			c <- 'x'
			c <- '\n'
			time.Sleep(10 * time.Microsecond)
			for _, c := range tt.input {
				pdp.cpu.Input <- uint8(c)
			}
		}()
		for i := 0; i < tt.cycles; i++ {
			pdp.Step()
		}
	}
}
