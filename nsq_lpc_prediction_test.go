package silk

import (
	"math"
	"math/rand"
	"testing"
)

var nsqLPCBenchmarkResult int32

func originalNSQLPCPrediction(history []int32, index int, coefficients *[MAX_LPC_ORDER / 2]int32, order int32) int32 {
	a := coefficients[0]
	sum := SMULWB(history[index], a)
	sum = SMLAWT(sum, history[index-1], a)
	a = coefficients[1]
	sum = SMLAWB(sum, history[index-2], a)
	sum = SMLAWT(sum, history[index-3], a)
	a = coefficients[2]
	sum = SMLAWB(sum, history[index-4], a)
	sum = SMLAWT(sum, history[index-5], a)
	a = coefficients[3]
	sum = SMLAWB(sum, history[index-6], a)
	sum = SMLAWT(sum, history[index-7], a)
	a = coefficients[4]
	sum = SMLAWB(sum, history[index-8], a)
	sum = SMLAWT(sum, history[index-9], a)
	for j := int32(10); j < order; j += 2 {
		a = coefficients[j>>1]
		sum = SMLAWB(sum, history[index-int(j)], a)
		sum = SMLAWT(sum, history[index-int(j)-1], a)
	}
	return sum
}

func TestNSQLPCPredictionCandidate(t *testing.T) {
	rng := rand.New(rand.NewSource(68921))
	for _, order := range []int32{16} {
		for trial := 0; trial < 256; trial++ {
			var h [MAX_FRAME_LENGTH/NB_SUBFR + NSQ_LPC_BUF_LENGTH]int32
			var packed [MAX_LPC_ORDER / 2]int32
			var wide [MAX_LPC_ORDER]int64
			for i := range h {
				h[i] = int32(rng.Uint32())
			}
			for i := 0; i < int(order); i++ {
				wide[i] = int64(int16(rng.Uint32()))
			}
			if trial%3 == 0 {
				for i := range h {
					h[i] = []int32{math.MinInt32, math.MaxInt32, -1, 0, 1}[i%5]
				}
				for i := 0; i < int(order); i++ {
					wide[i] = int64([]int16{math.MinInt16, math.MaxInt16, -1, 0, 1}[(i+trial)%5])
				}
			}
			for i := 0; i < int(order); i += 2 {
				packed[i/2] = int32(uint16(wide[i])) | int32(wide[i+1])<<16
			}
			for _, index := range []int{NSQ_LPC_BUF_LENGTH - 1, NSQ_LPC_BUF_LENGTH + 17, len(h) - 1} {
				want := originalNSQLPCPrediction(h[:], index, &packed, order)
				got := nsqLPCPrediction16(h[:], index, &wide)
				if got != want {
					t.Fatalf("order=%d trial=%d index=%d got=%d want=%d", order, trial, index, got, want)
				}
			}
		}
	}
}

func BenchmarkNSQLPCPrediction(b *testing.B) {
	var h [MAX_FRAME_LENGTH/NB_SUBFR + NSQ_LPC_BUF_LENGTH]int32
	var packed [MAX_LPC_ORDER / 2]int32
	var wide [MAX_LPC_ORDER]int64
	for i := range h {
		h[i] = int32(i * 0x36bad113)
	}
	for i := range wide {
		wide[i] = int64(int16(i*7993 - 4171))
	}
	for i := 0; i < MAX_LPC_ORDER; i += 2 {
		packed[i/2] = int32(uint16(wide[i])) | int32(wide[i+1])<<16
	}
	for _, name := range []string{"16/original", "16/grouped"} {
		b.Run(name, func(b *testing.B) {
			var sum int32
			order := int32(10)
			if name[:2] == "16" {
				order = 16
			}
			if name[3:] == "original" {
				for i := 0; i < b.N; i++ {
					sum += originalNSQLPCPrediction(h[:], NSQ_LPC_BUF_LENGTH-1+i%120, &packed, order)
				}
			} else {
				for i := 0; i < b.N; i++ {
					sum += nsqLPCPrediction16(h[:], NSQ_LPC_BUF_LENGTH-1+i%120, &wide)
				}
			}
			nsqLPCBenchmarkResult = sum
		})
	}
}

func originalNSQShape16(input int32, state *[MAX_SHAPE_LPC_ORDER]int32, coefficients *[MAX_SHAPE_LPC_ORDER]int16, warp int32) int32 {
	tmp2 := SMLAWB(input, state[0], warp)
	tmp1 := SMLAWB(state[0], state[1]-tmp2, warp)
	state[0] = tmp2
	sum := SMULWB(tmp2, int32(coefficients[0]))
	for j := 2; j < MAX_SHAPE_LPC_ORDER; j += 2 {
		tmp2 = SMLAWB(state[j-1], state[j]-tmp1, warp)
		state[j-1] = tmp1
		sum = SMLAWB(sum, tmp1, int32(coefficients[j-1]))
		tmp1 = SMLAWB(state[j], state[j+1]-tmp2, warp)
		state[j] = tmp2
		sum = SMLAWB(sum, tmp2, int32(coefficients[j]))
	}
	state[MAX_SHAPE_LPC_ORDER-1] = tmp1
	return SMLAWB(sum, tmp1, int32(coefficients[MAX_SHAPE_LPC_ORDER-1]))
}

func TestNSQShape16Candidate(t *testing.T) {
	rng := rand.New(rand.NewSource(53468))
	for trial := 0; trial < 1024; trial++ {
		var state [MAX_SHAPE_LPC_ORDER]int32
		var narrow [MAX_SHAPE_LPC_ORDER]int16
		var wide [MAX_SHAPE_LPC_ORDER]int64
		for i := range state {
			state[i] = int32(rng.Uint32())
			narrow[i] = int16(rng.Uint32())
			wide[i] = int64(narrow[i])
		}
		original := state
		input, warp := int32(rng.Uint32()), int32(rng.Uint32())
		if trial%3 == 0 {
			warp = []int32{0, 1, -1, math.MinInt16, math.MaxInt16}[trial%5]
		}
		want := originalNSQShape16(input, &original, &narrow, warp)
		got := nsqShape16(input, &state, &wide, int64(int16(warp)))
		if got != want || state != original {
			t.Fatalf("trial=%d output or shape history differs", trial)
		}
	}
}

func BenchmarkNSQShape16(b *testing.B) {
	for _, name := range []string{"original", "expanded"} {
		b.Run(name, func(b *testing.B) {
			var state [MAX_SHAPE_LPC_ORDER]int32
			var narrow [MAX_SHAPE_LPC_ORDER]int16
			var wide [MAX_SHAPE_LPC_ORDER]int64
			for i := range state {
				state[i] = int32(i * 0x71839)
				narrow[i] = int16(i*8973 - 4111)
				wide[i] = int64(narrow[i])
			}
			var sum int32
			b.ResetTimer()
			if name == "original" {
				for i := 0; i < b.N; i++ {
					sum += originalNSQShape16(3768154, &state, &narrow, 21368)
				}
			} else {
				for i := 0; i < b.N; i++ {
					sum += nsqShape16(3768154, &state, &wide, 21368)
				}
			}
			nsqLPCBenchmarkResult = sum
		})
	}
}
