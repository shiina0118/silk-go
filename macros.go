package silk

import (
	"math"
	"math/bits"
)

func SMULWB(a32, b32 int32) int32 {
	return int32((int64(a32) * int64(int16(b32))) >> 16)
}

func SMLAWB(a32, b32, c32 int32) int32 {
	return a32 + SMULWB(b32, c32)
}

func SMULWT(a32, b32 int32) int32 {
	return int32((int64(a32) * int64(b32>>16)) >> 16)
}

func SMLAWT(a32, b32, c32 int32) int32 {
	return a32 + SMULWT(b32, c32)
}

func SMULBB(a32, b32 int32) int32 {
	return int32(int16(a32)) * int32(int16(b32))
}

func SMLABB(a32, b32, c32 int32) int32 {
	b32, c32 = int32(int16(b32)), int32(int16(c32))
	return a32 + (b32 * c32)
}

func SMLABT(a32, b32, c32 int32) int32 {
	return a32 + int32(int16(b32))*(c32>>16)
}

func SMULWW(a32, b32 int32) int32 {
	return int32((int64(a32) * int64(b32)) >> 16)
}

func SMLAWW(a32, b32, c32 int32) int32 {
	return a32 + SMULWW(b32, c32)
}

func SMMUL(a32, b32 int32) int32 {
	return int32(RSHIFT64(SMULL(a32, b32), 32))
}

func ADD_SAT32(a, b int32) int32 {
	if (int(a+b) & 0x80000000) == 0 {
		if (int(a&b) & 0x80000000) != 0 {
			return math.MinInt32
		}
	} else if (int(a|b) & 0x80000000) == 0 {
		return math.MaxInt32
	}
	return a + b
}

func SUB_SAT32(a, b int32) int32 {
	if int(a-b)&0x80000000 == 0 {
		if int(a)&(int(b)^0x80000000)&0x80000000 != 0 {
			return math.MinInt32
		}
	} else if ((int(a) ^ 0x80000000) & int(b) & 0x80000000) != 0 {
		return math.MaxInt32
	}
	return a - b
}

func CLZ16(in16 uint16) uint16 {
	return uint16(bits.LeadingZeros16(in16))
}

func CLZ32(in32 int32) int32 {
	return int32(bits.LeadingZeros32(uint32(in32)))
}
