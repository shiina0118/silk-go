package silk

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"testing"
)

func TestPitchDownsamplersMatchOriginal(t *testing.T) {
	for _, kernel := range []struct {
		name                string
		stateSize           int
		outputsPerThree     int
		optimized, original func(*slice[int32], *slice[int16], *slice[int16], int32)
	}{
		{"down3", 8, 1, resampler_down3, originalResamplerDown3},
		{"down2_3", 6, 2, resampler_down2_3, originalResamplerDown23},
	} {
		for _, length := range []int{0, 1, 2, 3, 4, 5, 6, 479, 480, 481, 482, 483, 959, 960, 961, 1923, 4099} {
			for _, offset := range []int{0, 1, 7, 19} {
				for _, pattern := range []string{"silence", "extremes", "impulses", "random"} {
					t.Run(fmt.Sprintf("%s/%s/length=%d/offset=%d", kernel.name, pattern, length, offset), func(t *testing.T) {
						rng := rand.New(rand.NewSource(int64(length*113 + offset)))
						state := make([]int32, offset+kernel.stateSize+9)
						for i := range state {
							state[i] = int32(rng.Uint32())
						}
						originalState := slices.Clone(state)
						input := make([]int16, offset+length+11)
						for i := range input {
							switch pattern {
							case "extremes":
								input[i] = []int16{math.MinInt16, math.MaxInt16, -1, 0, 1}[i%5]
							case "impulses":
								if i%31 == 0 {
									input[i] = math.MinInt16
								}
								if i%43 == 0 {
									input[i] = math.MaxInt16
								}
							case "random":
								input[i] = int16(rng.Uint32())
							}
						}
						inputBefore := slices.Clone(input)
						output := make([]int16, offset+length*kernel.outputsPerThree/3+17)
						for i := range output {
							output[i] = 12345
						}
						originalOutput := slices.Clone(output)
						kernel.optimized(mem2Slice(state).off(offset), mem2Slice(output).off(offset), mem2Slice(input).off(offset), int32(length))
						kernel.original(mem2Slice(originalState).off(offset), mem2Slice(originalOutput).off(offset), mem2Slice(input).off(offset), int32(length))
						if !slices.Equal(state, originalState) {
							t.Fatal("filter state differs from original")
						}
						if !slices.Equal(output, originalOutput) {
							t.Fatal("output or output guard differs from original")
						}
						if !slices.Equal(input, inputBefore) {
							t.Fatal("input was changed")
						}
					})
				}
			}
		}
		t.Run(kernel.name+"/successive-calls", func(t *testing.T) {
			rng := rand.New(rand.NewSource(77))
			state, originalState := make([]int32, kernel.stateSize+7), make([]int32, kernel.stateSize+7)
			for call := 0; call < 64; call++ {
				length := rng.Intn(1447)
				input := make([]int16, length+13)
				for i := range input {
					input[i] = int16(rng.Uint32())
				}
				output := make([]int16, length*kernel.outputsPerThree/3+19)
				originalOutput := slices.Clone(output)
				kernel.optimized(mem2Slice(state).off(3), mem2Slice(output).off(5), mem2Slice(input).off(7), int32(length))
				kernel.original(mem2Slice(originalState).off(3), mem2Slice(originalOutput).off(5), mem2Slice(input).off(7), int32(length))
				if !slices.Equal(state, originalState) || !slices.Equal(output, originalOutput) {
					t.Fatalf("state or output differs after call %d", call)
				}
			}
		})
	}
}

func TestResamplerPrivateAR2MatchesOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(98))
	for _, length := range []int{0, 1, 2, 7, 479, 480, 961} {
		for _, offset := range []int{0, 1, 7} {
			for _, alias := range []bool{false, true} {
				state := make([]int32, length+offset+13)
				for i := range state {
					state[i] = int32(rng.Uint32())
				}
				originalState := slices.Clone(state)
				input := make([]int16, length+offset+3)
				for i := range input {
					input[i] = int16(rng.Uint32())
				}
				coefs := []int16{-32768, 32767, -123, 789}
				output := make([]int32, length+offset+3)
				originalOutput := slices.Clone(output)
				if alias {
					output, originalOutput = state, originalState
				}
				resampler_private_AR2(mem2Slice(state).off(offset), mem2Slice(output).off(offset), mem2Slice(input).off(offset), mem2Slice(coefs).off(1), int32(length))
				originalResamplerAR2(mem2Slice(originalState).off(offset), mem2Slice(originalOutput).off(offset), mem2Slice(input).off(offset), mem2Slice(coefs).off(1), int32(length))
				if !slices.Equal(state, originalState) || !slices.Equal(output, originalOutput) {
					t.Fatalf("AR2 differs: length=%d, offset=%d, alias=%t", length, offset, alias)
				}
			}
		}
	}
}

func BenchmarkPitchDownsample24KHz(b *testing.B) {
	for _, kernel := range []struct {
		name string
		fn   func(*slice[int32], *slice[int16], *slice[int16], int32)
	}{
		{"optimized", resampler_down3},
		{"original", originalResamplerDown3},
	} {
		b.Run(kernel.name, func(b *testing.B) {
			var state [8]int32
			var input [960]int16
			var output [320]int16
			for i := range input {
				input[i] = int16(i * 73)
			}
			s, in, out := mem2Slice(state[:]), mem2Slice(input[:]), mem2Slice(output[:])
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				kernel.fn(s, out, in, int32(len(input)))
			}
		})
	}
}

// Frozen wrapper-based kernels from a3492666bd20310e7aa844980558ff83748b395a.
// Their arithmetic and buffer movement are kept independent of the new kernels.

func originalResamplerDown3(S *slice[int32], out, in *slice[int16], inLen int32) {
	var (
		nSamplesIn, counter, res_Q6 int32

		buf     = alloc[int32](RESAMPLER_MAX_BATCH_SIZE_IN + ORDER_FIR)
		buf_ptr *slice[int32]
	)

	S.copy(buf, ORDER_FIR)

	coefs := mem2Slice[int16](Resampler_1_3_COEFS_LQ)

	for {
		nSamplesIn = min(inLen, RESAMPLER_MAX_BATCH_SIZE_IN)

		originalResamplerAR2(S.off(ORDER_FIR), buf.off(ORDER_FIR), in, coefs, nSamplesIn)

		buf_ptr = buf.off(0)
		counter = nSamplesIn
		for counter > 2 {
			res_Q6 = SMULWB(ADD32(buf_ptr.idx(0), buf_ptr.idx(5)), int32(coefs.idx(2)))
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr.idx(1), buf_ptr.idx(4)), int32(coefs.idx(3)))
			res_Q6 = SMLAWB(res_Q6, ADD32(buf_ptr.idx(2), buf_ptr.idx(3)), int32(coefs.idx(4)))

			*out.ptr(0) = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			out = out.off(1)

			buf_ptr = buf_ptr.off(3)
			counter -= 3
		}

		in = in.off(int(nSamplesIn))
		inLen -= nSamplesIn

		if inLen > 0 {
			buf.off(int(nSamplesIn)).copy(buf, ORDER_FIR)
		} else {
			break
		}
	}

	buf.off(int(nSamplesIn)).copy(S, ORDER_FIR)
}

func originalResamplerDown23(S *slice[int32], out, in *slice[int16], inLen int32) {
	var (
		nSamplesIn, counter, res_Q6 int32
		buf                         = alloc[int32](RESAMPLER_MAX_BATCH_SIZE_IN + DOWN23_ORDER_FIR)
		buf_ptr                     *slice[int32]
	)

	S.copy(buf, DOWN23_ORDER_FIR)

	coefs := mem2Slice[int16](Resampler_2_3_COEFS_LQ)

	for {
		nSamplesIn = min(inLen, RESAMPLER_MAX_BATCH_SIZE_IN)

		originalResamplerAR2(S.off(DOWN23_ORDER_FIR), buf.off(DOWN23_ORDER_FIR), in,
			coefs.off(0), nSamplesIn)

		buf_ptr = buf.off(0)
		counter = nSamplesIn
		for counter > 2 {
			res_Q6 = SMULWB(buf_ptr.idx(0), int32(coefs.idx(2)))
			res_Q6 = SMLAWB(res_Q6, buf_ptr.idx(1), int32(coefs.idx(3)))
			res_Q6 = SMLAWB(res_Q6, buf_ptr.idx(2), int32(coefs.idx(5)))
			res_Q6 = SMLAWB(res_Q6, buf_ptr.idx(3), int32(coefs.idx(4)))

			*out.ptr(0) = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			out = out.off(1)

			res_Q6 = SMULWB(buf_ptr.idx(1), int32(coefs.idx(4)))
			res_Q6 = SMLAWB(res_Q6, buf_ptr.idx(2), int32(coefs.idx(5)))
			res_Q6 = SMLAWB(res_Q6, buf_ptr.idx(3), int32(coefs.idx(3)))
			res_Q6 = SMLAWB(res_Q6, buf_ptr.idx(4), int32(coefs.idx(2)))

			*out.ptr(0) = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			out = out.off(1)

			buf_ptr = buf_ptr.off(3)
			counter -= 3
		}

		in = in.off(int(nSamplesIn))
		inLen -= nSamplesIn

		if inLen > 0 {
			buf.off(int(nSamplesIn)).copy(buf, DOWN23_ORDER_FIR)
		} else {
			break
		}
	}

	buf.off(int(nSamplesIn)).copy(S, DOWN23_ORDER_FIR)
}

func originalResamplerAR2(S, out_Q8 *slice[int32], in, A_Q14 *slice[int16], inLen int32) {
	var k, out32 int32

	for k = 0; k < inLen; k++ {
		out32 = ADD_LSHIFT32(S.idx(0), int32(in.idx(int(k))), 8)
		*out_Q8.ptr(int(k)) = out32
		out32 = LSHIFT(out32, 2)
		*S.ptr(0) = SMLAWB(S.idx(1), out32, int32(A_Q14.idx(0)))
		*S.ptr(1) = SMULWB(out32, int32(A_Q14.idx(1)))
	}
}
