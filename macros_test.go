package silk

import (
	"math"
	"math/rand"
	"testing"
)

// Reference formulas are the original 32-bit decomposed fixed-point macros.
// These comparisons cover sign extension, rounding boundaries and wraparound.
func TestWideFixedPointMultiply(t *testing.T) {
	values := []int32{math.MinInt32, math.MinInt32 + 1, -1073741824, -65537, -65536, -65535,
		-32769, -32768, -32767, -2, -1, 0, 1, 2, 32767, 32768, 32769, 65535, 65536, 65537,
		1073741824, math.MaxInt32 - 1, math.MaxInt32}
	check := func(a, b, c int32) {
		t.Helper()
		wb := func(x, y int32) int32 { y = int32(int16(y)); return (x>>16)*y + (((x & 0xffff) * y) >> 16) }
		wt := func(x, y int32) int32 { return (x>>16)*(y>>16) + (((x & 0xffff) * (y >> 16)) >> 16) }
		ww := func(x, y int32) int32 { return wb(x, y) + x*(((y>>15)+1)>>1) }
		for _, test := range []struct {
			name      string
			got, want int32
		}{
			{"SMULWB", SMULWB(a, b), wb(a, b)}, {"SMLAWB", SMLAWB(a, b, c), a + wb(b, c)},
			{"SMULWT", SMULWT(a, b), wt(a, b)}, {"SMLAWT", SMLAWT(a, b, c), a + wt(b, c)},
			{"SMULWW", SMULWW(a, b), ww(a, b)}, {"SMLAWW", SMLAWW(a, b, c), a + ww(b, c)},
		} {
			if test.got != test.want {
				t.Fatalf("%s(%d,%d,%d) = %d, want %d", test.name, a, b, c, test.got, test.want)
			}
		}
	}
	for _, a := range values {
		for _, b := range values {
			for _, c := range values {
				check(a, b, c)
			}
		}
	}
	rng := rand.New(rand.NewSource(417))
	for i := 0; i < 100000; i++ {
		check(int32(rng.Uint32()), int32(rng.Uint32()), int32(rng.Uint32()))
	}
}

func TestLeadingZeros(t *testing.T) {
	reference := func(value uint32, width int) int {
		count := 0
		for bit := width - 1; bit >= 0 && value&(1<<uint(bit)) == 0; bit-- {
			count++
		}
		return count
	}
	for i := 0; i <= 65535; i++ {
		if got, want := CLZ16(uint16(i)), uint16(reference(uint32(i), 16)); got != want {
			t.Fatalf("CLZ16(%d)=%d want %d", i, got, want)
		}
	}
	rng := rand.New(rand.NewSource(881))
	for i := 0; i < 100000; i++ {
		value := rng.Uint32()
		if got, want := CLZ32(int32(value)), int32(reference(value, 32)); got != want {
			t.Fatalf("CLZ32(%d)=%d want %d", value, got, want)
		}
	}
	if CLZ32(0) != 32 {
		t.Fatal("CLZ32(0)")
	}
}
